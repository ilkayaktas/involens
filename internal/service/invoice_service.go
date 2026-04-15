package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/involens/invoice-ocr/internal/imageutil"
	"github.com/involens/invoice-ocr/internal/llm"
	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/repository"
	"github.com/involens/invoice-ocr/internal/retry"
)

// ErrLLMExtraction is returned when the LLM extractor fails after all retries.
type ErrLLMExtraction struct{ Cause error }

func (e *ErrLLMExtraction) Error() string { return "llm extraction failed: " + e.Cause.Error() }
func (e *ErrLLMExtraction) Unwrap() error { return e.Cause }

// invoiceRepo is the minimal repository interface needed by InvoiceService.
// The concrete *repository.InvoiceRepository satisfies this interface.
type invoiceRepo interface {
	Create(ctx context.Context, inv *model.Invoice) (*model.Invoice, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.Invoice, error)
	GetByImageHash(ctx context.Context, hash string) (*model.Invoice, error)
	List(ctx context.Context, skip, limit int64) ([]*model.Invoice, error)
	Search(ctx context.Context, params repository.SearchParams) ([]*model.Invoice, int64, error)
	GetStats(ctx context.Context) (*repository.Stats, error)
}

// InvoiceService orchestrates invoice ingestion and retrieval.
type InvoiceService struct {
	repo        invoiceRepo
	extractor   llm.InvoiceExtractor
	storagePath string
}

// New creates a new InvoiceService.
// repo must satisfy invoiceRepo (e.g. *repository.InvoiceRepository does).
func New(repo invoiceRepo, extractor llm.InvoiceExtractor, storagePath string) *InvoiceService {
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

	mimeType := mimeTypeFromFilename(header.Filename)
	return s.ProcessInvoiceData(ctx, imageData, mimeType, header.Filename)
}

// ProcessInvoiceData saves the provided image bytes, calls the LLM extractor, and persists the result.
// It accepts raw bytes directly, making it suitable for async processing pipelines.
// Idempotency: if an invoice with the same SHA-256 image hash already exists, it is returned immediately.
func (s *InvoiceService) ProcessInvoiceData(ctx context.Context, imageData []byte, mimeType string, filename string) (*model.Invoice, error) {
	// Compute SHA-256 hash for idempotency check.
	hashBytes := sha256.Sum256(imageData)
	imageHash := fmt.Sprintf("%x", hashBytes)

	// Check if we have already processed this exact image.
	if existing, err := s.repo.GetByImageHash(ctx, imageHash); err == nil {
		return existing, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("service: idempotency check: %w", err)
	}

	// Persist image to storage.
	imagePath, err := s.saveImage(imageData, filename)
	if err != nil {
		return nil, fmt.Errorf("service: save image: %w", err)
	}

	// Resize image for LLM processing (longest edge capped at 1568px).
	resizedData, resizedMime, err := imageutil.ResizeForLLM(imageData, mimeType, imageutil.DefaultMaxPx)
	if err != nil {
		return nil, fmt.Errorf("service: resize image: %w", err)
	}

	// Call LLM extractor with retry (3 total attempts, exponential backoff + jitter).
	var extracted *model.ExtractedInvoice
	extractErr := retry.Do(ctx, 3, time.Second, 500*time.Millisecond, func() error {
		var e error
		extracted, e = s.extractor.ExtractInvoice(ctx, resizedData, resizedMime)
		return e
	})
	if extractErr != nil {
		// Persist a failed record so the operator can retry.
		now := time.Now().UTC()
		inv := &model.Invoice{
			InvoiceNumber: fmt.Sprintf("failed-%d", now.UnixNano()),
			ImagePath:     imagePath,
			ImageHash:     imageHash,
			Status:        model.StatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if _, saveErr := s.repo.Create(ctx, inv); saveErr != nil {
			_ = saveErr
		}
		return nil, &ErrLLMExtraction{Cause: extractErr}
	}

	// Map extracted data to the Invoice document.
	inv := mapExtractedToInvoice(extracted, imagePath, imageHash)

	// Cross-validate totals; degrade confidence on mismatch.
	crossValidate(inv)

	created, err := s.repo.Create(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("service: persist invoice: %w", err)
	}

	return created, nil
}

// crossValidate checks line-item sums and total consistency, degrading confidence if needed.
func crossValidate(inv *model.Invoice) {
	const tolerance = 0.01 // 1%

	var issues []string

	// 1. Sum of line items vs subtotal.
	if len(inv.LineItems) > 0 {
		var lineSum float64
		for _, li := range inv.LineItems {
			lineSum += li.Amount
		}
		if inv.Subtotal != 0 && !withinTolerance(lineSum, inv.Subtotal, tolerance) {
			issues = append(issues, fmt.Sprintf(
				"line items sum (%.2f) does not match subtotal (%.2f)",
				lineSum, inv.Subtotal,
			))
		}
	}

	// 2. total == subtotal + tax_amount.
	expectedTotal := inv.Subtotal + inv.TaxAmount
	if inv.Total != 0 && !withinTolerance(inv.Total, expectedTotal, tolerance) {
		issues = append(issues, fmt.Sprintf(
			"total (%.2f) does not equal subtotal + tax_amount (%.2f + %.2f = %.2f)",
			inv.Total, inv.Subtotal, inv.TaxAmount, expectedTotal,
		))
	}

	if len(issues) > 0 {
		inv.Confidence = model.ConfidenceLow
		inv.Status = model.StatusReview
		note := "cross-validation failed: "
		for i, iss := range issues {
			if i > 0 {
				note += "; "
			}
			note += iss
		}
		if inv.Notes != nil && *inv.Notes != "" {
			combined := *inv.Notes + " | " + note
			inv.Notes = &combined
		} else {
			inv.Notes = &note
		}
	}
}

// withinTolerance returns true when |a-b| / max(|b|, 1) <= pct.
func withinTolerance(a, b, pct float64) bool {
	denom := math.Abs(b)
	if denom < 1 {
		denom = 1
	}
	return math.Abs(a-b)/denom <= pct
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

// SearchInvoices queries invoices with filters, sorting, and pagination.
func (s *InvoiceService) SearchInvoices(ctx context.Context, params repository.SearchParams) ([]*model.Invoice, int64, error) {
	if params.Limit <= 0 || params.Limit > 100 {
		params.Limit = 20
	}
	return s.repo.Search(ctx, params)
}

// GetStats returns aggregate statistics for the invoice collection.
func (s *InvoiceService) GetStats(ctx context.Context) (*repository.Stats, error) {
	return s.repo.GetStats(ctx)
}

// --- helpers ---

func (s *InvoiceService) saveImage(data []byte, filename string) (string, error) {
	if err := os.MkdirAll(s.storagePath, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(s.storagePath, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(filename)))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

func mapExtractedToInvoice(e *model.ExtractedInvoice, imagePath, imageHash string) *model.Invoice {
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
		ImageHash:     imageHash,
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
