package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/involens/invoice-ocr/internal/service"
)

// APIHandler handles HTTP requests for the read-only API service.
type APIHandler struct {
	svc *service.InvoiceService
}

// NewAPIHandler creates an APIHandler.
func NewAPIHandler(svc *service.InvoiceService) *APIHandler {
	return &APIHandler{svc: svc}
}

// RegisterRoutes attaches routes to the provided gin Engine.
func (h *APIHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Health)

	v1 := r.Group("/v1")
	{
		v1.GET("/invoices", h.ListInvoices)
		v1.GET("/invoices/:id", h.GetInvoice)
	}
}

// Health returns a simple liveness response.
func (h *APIHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "api",
	})
}

// ListInvoices returns a paginated list of invoices.
//
// GET /v1/invoices?skip=0&limit=20
func (h *APIHandler) ListInvoices(c *gin.Context) {
	skip, _ := strconv.ParseInt(c.DefaultQuery("skip", "0"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)

	invoices, err := h.svc.ListInvoices(c.Request.Context(), skip, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  invoices,
		"skip":  skip,
		"limit": limit,
	})
}

// GetInvoice returns a single invoice by its ID.
//
// GET /v1/invoices/:id
func (h *APIHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")

	inv, err := h.svc.GetInvoice(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, inv)
}
