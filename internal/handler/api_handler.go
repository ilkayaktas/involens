package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/involens/invoice-ocr/internal/repository"
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
// Note: /v1/invoices/search and /v1/invoices/stats are registered BEFORE
// /v1/invoices/:id so Gin does not treat "search"/"stats" as :id values.
func (h *APIHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Health)

	v1 := r.Group("/v1")
	{
		v1.GET("/invoices", h.ListInvoices)
		v1.GET("/invoices/search", h.SearchInvoices)
		v1.GET("/invoices/stats", h.GetStats)
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

// buildSearchParams parses common query parameters into a SearchParams struct.
func buildSearchParams(c *gin.Context) repository.SearchParams {
	skip, _ := strconv.ParseInt(c.DefaultQuery("skip", "0"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	minAmount, _ := strconv.ParseFloat(c.DefaultQuery("min_amount", "0"), 64)
	maxAmount, _ := strconv.ParseFloat(c.DefaultQuery("max_amount", "0"), 64)

	return repository.SearchParams{
		Vendor:    c.Query("vendor"),
		From:      c.Query("from"),
		To:        c.Query("to"),
		MinAmount: minAmount,
		MaxAmount: maxAmount,
		Status:    c.Query("status"),
		Currency:  c.Query("currency"),
		SortBy:    c.DefaultQuery("sort", "created_at"),
		SortOrder: c.DefaultQuery("order", "desc"),
		Skip:      skip,
		Limit:     limit,
	}
}

// ListInvoices returns a paginated list of invoices with optional filters.
//
// GET /v1/invoices?vendor=Acme&from=2026-01-01&to=2026-03-31
//
//	&min_amount=1000&max_amount=5000&status=processed&currency=TRY
//	&sort=date&order=desc&skip=0&limit=20
func (h *APIHandler) ListInvoices(c *gin.Context) {
	params := buildSearchParams(c)

	invoices, total, err := h.svc.SearchInvoices(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  invoices,
		"total": total,
		"skip":  params.Skip,
		"limit": params.Limit,
	})
}

// SearchInvoices is an alias for ListInvoices mounted at /v1/invoices/search.
//
// GET /v1/invoices/search?...  (same query params as ListInvoices)
func (h *APIHandler) SearchInvoices(c *gin.Context) {
	h.ListInvoices(c)
}

// GetInvoice returns a single invoice by its ObjectID hex string.
//
// GET /v1/invoices/:id
func (h *APIHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")

	inv, err := h.svc.GetInvoice(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, inv)
}

// GetStats returns aggregate statistics for the invoice collection.
//
// GET /v1/invoices/stats
func (h *APIHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}
