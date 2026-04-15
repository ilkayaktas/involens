package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/involens/invoice-ocr/internal/imageutil"
	"github.com/involens/invoice-ocr/internal/llm"
	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/repository"
)

// InvoiceService orchestrates invoice ingestion and retrieval.
type InvoiceService struct {
	repo        *repository.InvoiceRepository
	extractor   llm.InvoiceExtractor
	storagePath string
}

// New creates a new InvoiceService.
func New(repo *repository.InvoiceRepository, extractor llm.InvoiceExtractor, storagePath string) *InvoiceService {
	return &InvoiceService{
		repo:        repo,
		extractor:   extractor,
		storagePath: storagePath,
	}
}

// ProcessInvoice saves the uploaded image, calls the LLM extractor, and persists the result.
func (s *InvoiceService) ProcessInvoice(ctx context.Context, file multipart.File, header *multipart.FileHeader) (*model.Invoice, error) {
	// Read image bytes.
	imageData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("service: read image: %w", err)
	}

	// Persist image to storage.
	imagePath, err := s.saveImage(imageData, header.Filename)
	if err != nil {
		return nil, fmt.Errorf("service: save image: %w", err)
	}

	// Determine MIME type from extension.
	mimeType := mimeTypeFromFilename(header.Filename)

	// Resize image for LLM processing (longest edge capped at 1568px).
	resizedData, resizedMime, err := imageutil.ResizeForLLM(imageData, mimeType, imageutil.DefaultMaxPx)
	if err != nil {
		return nil, fmt.Errorf("service: resize image: %w", err)
	}

	// Call LLM extractor.
	extracted, err := s.extractor.ExtractInvoice(ctx, resizedData, resizedMime)
	if err != nil {
		// Persist a failed record so the operator can retry.
		// Use a unique invoice_number to avoid duplicate-key conflicts on the unique index.
		now := time.Now().UTC()
		inv := &model.Invoice{
			InvoiceNumber: fmt.Sprintf("failed-%d", now.UnixNano()),
			ImagePath:     imagePath,
			Status:        model.StatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if _, saveErr := s.repo.Create(ctx, inv); saveErr != nil {
			// Log but don't mask the original extraction error.
			_ = saveErr
		}
		return nil, fmt.Errorf("service: extract invoice: %w", err)
	}

	// Map extracted data to the Invoice document.
	inv := mapExtractedToInvoice(extracted, imagePath)

	created, err := s.repo.Create(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("service: persist invoice: %w", err)
	}

	return created, nil
}

// GetInvoice returns a single invoice by hex ID string.
func (s *InvoiceService) GetInvoice(ctx context.Context, idStr string) (*model.Invoice, error) {
	id, err := bson.ObjectIDFromHex(idStr)
	if err != nil {
		return nil, fmt.Errorf("service: invalid id %q: %w", idStr, err)
	}
	return s.repo.GetByID(ctx, id)
}

// ListInvoices returns a paginated list of invoices. Limit is capped at 100.
func (s *InvoiceService) ListInvoices(ctx context.Context, skip, limit int64) ([]*model.Invoice, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	return s.repo.List(ctx, skip, limit)
}

// --- helpers ---

func (s *InvoiceService) saveImage(data []byte, filename string) (string, error) {
	if err := os.MkdirAll(s.storagePath, 0o755); err != nil {
		return "", err
	}
	// Prefix with timestamp to avoid collisions.
	dest := filepath.Join(s.storagePath, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(filename)))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

func mapExtractedToInvoice(e *model.ExtractedInvoice, imagePath string) *model.Invoice {
	return &model.Invoice{
		InvoiceNumber: e.InvoiceNumber,
		Vendor:        e.Vendor,
		Customer:      e.Customer,
		Date:          e.Date,
		DueDate:       e.DueDate,
		Currency:      e.Currency,
		LineItems:     e.LineItems,
		Subtotal:      e.Subtotal,
		TaxRate:       e.TaxRate,
		TaxAmount:     e.TaxAmount,
		Total:         e.Total,
		Notes:         e.Notes,
		RawResponse:   e.RawResponse,
		ImagePath:     imagePath,
		Confidence:    e.Confidence,
		Status:        model.StatusProcessed,
	}
}

func mimeTypeFromFilename(name string) string {
	switch filepath.Ext(name) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
