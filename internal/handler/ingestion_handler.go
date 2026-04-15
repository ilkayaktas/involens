package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/involens/invoice-ocr/internal/service"
)

// IngestionHandler handles HTTP requests for invoice ingestion.
type IngestionHandler struct {
	svc *service.InvoiceService
}

// NewIngestionHandler creates an IngestionHandler.
func NewIngestionHandler(svc *service.InvoiceService) *IngestionHandler {
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
	file, header, err := c.Request.FormFile("invoice")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'invoice' file field: " + err.Error()})
		return
	}
	defer file.Close()

	inv, err := h.svc.ProcessInvoice(c.Request.Context(), file, header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, inv)
}

