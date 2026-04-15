package handler

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/service"
)

const maxUploadBytes = 10 << 20 // 10 MB

// allowedMIMETypes lists the image formats accepted by the ingestion endpoint.
var allowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// InvoiceProcessor is the minimal interface the handler needs from the service layer.
type InvoiceProcessor interface {
	ProcessInvoice(ctx context.Context, file multipart.File, header *multipart.FileHeader) (*model.Invoice, error)
}

// invoiceResponse is the shape returned on a successful 201 Created.
type invoiceResponse struct {
	ID            string      `json:"id"`
	InvoiceNumber string      `json:"invoice_number"`
	Status        string      `json:"status"`
	Confidence    string      `json:"confidence"`
	Total         float64     `json:"total"`
	Currency      string      `json:"currency"`
	Vendor        vendorBrief `json:"vendor"`
}

type vendorBrief struct {
	Name string `json:"name"`
}

// IngestionHandler handles HTTP requests for invoice ingestion.
type IngestionHandler struct {
	svc InvoiceProcessor
}

// NewIngestionHandler creates an IngestionHandler.
func NewIngestionHandler(svc *service.InvoiceService) *IngestionHandler {
	return &IngestionHandler{svc: svc}
}

// newIngestionHandlerWithProcessor creates an IngestionHandler from any InvoiceProcessor.
// Used in tests.
func newIngestionHandlerWithProcessor(svc InvoiceProcessor) *IngestionHandler {
	return &IngestionHandler{svc: svc}
}

// RegisterRoutes attaches routes to the provided gin Engine.
func (h *IngestionHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Health)
	r.POST("/invoices", h.Upload)
}

// Health returns a simple liveness response.
func (h *IngestionHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "ingestion",
	})
}

// Upload accepts a multipart image upload, runs OCR, and returns the created invoice.
//
// POST /invoices
// Content-Type: multipart/form-data
// Field: "invoice" — the image file
func (h *IngestionHandler) Upload(c *gin.Context) {
	// Enforce overall request body size before parsing the multipart form.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	file, header, err := c.Request.FormFile("invoice")
	if err != nil {
		// MaxBytesReader sets the error when the body exceeds the limit.
		if isMaxBytesError(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image exceeds 10 MB size limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'invoice' file field: " + err.Error()})
		return
	}
	defer file.Close()

	// Validate file size via Content-Length header as an early check (advisory).
	if header.Size > maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image exceeds 10 MB size limit"})
		return
	}

	// Detect MIME type from the first 512 bytes.
	sniff := make([]byte, 512)
	n, readErr := file.Read(sniff)
	if readErr != nil && readErr != io.EOF {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file: " + readErr.Error()})
		return
	}
	detected := http.DetectContentType(sniff[:n])
	if !allowedMIMETypes[detected] {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": "unsupported image format: " + detected + "; accepted formats are JPEG, PNG, WebP, GIF",
		})
		return
	}

	// Seek back to start so the service reads the full file.
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset file reader: " + seekErr.Error()})
		return
	}

	inv, err := h.svc.ProcessInvoice(c.Request.Context(), file, header)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, toInvoiceResponse(inv))
}

// handleServiceError maps known service errors to appropriate HTTP status codes.
func (h *IngestionHandler) handleServiceError(c *gin.Context, err error) {
	var llmErr *service.ErrLLMExtraction
	if errors.As(err, &llmErr) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "LLM extraction failed: " + llmErr.Cause.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// isMaxBytesError checks whether err was produced by http.MaxBytesReader.
func isMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// toInvoiceResponse converts a full Invoice model to the abbreviated 201 response shape.
func toInvoiceResponse(inv *model.Invoice) invoiceResponse {
	return invoiceResponse{
		ID:            inv.ID.Hex(),
		InvoiceNumber: inv.InvoiceNumber,
		Status:        inv.Status,
		Confidence:    inv.Confidence,
		Total:         inv.Total,
		Currency:      inv.Currency,
		Vendor:        vendorBrief{Name: inv.Vendor.Name},
	}
}
