package handler

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/repository"
	"github.com/involens/invoice-ocr/internal/service"
	"github.com/involens/invoice-ocr/internal/worker"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/time/rate"
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

// asyncResponse is the shape returned on a successful 202 Accepted.
type asyncResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// IngestionHandler handles HTTP requests for invoice ingestion.
type IngestionHandler struct {
	svc      InvoiceProcessor
	asyncSvc worker.ProcessorService
	jobRepo  *repository.JobRepository
	pool     *worker.Pool
	limiter  *rate.Limiter
	db       *mongo.Database
}

// NewIngestionHandler creates an IngestionHandler for synchronous processing only.
func NewIngestionHandler(svc *service.InvoiceService, db *mongo.Database, rps, burst int) *IngestionHandler {
	return &IngestionHandler{
		svc:     svc,
		db:      db,
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

// NewIngestionHandlerWithAsync creates an IngestionHandler that supports both sync and async processing.
func NewIngestionHandlerWithAsync(svc *service.InvoiceService, jobRepo *repository.JobRepository, pool *worker.Pool, db *mongo.Database, rps, burst int) *IngestionHandler {
	return &IngestionHandler{
		svc:      svc,
		asyncSvc: svc,
		jobRepo:  jobRepo,
		pool:     pool,
		db:       db,
		limiter:  rate.NewLimiter(rate.Limit(rps), burst),
	}
}

// newIngestionHandlerWithProcessor creates an IngestionHandler from any InvoiceProcessor.
// Used in tests.
func newIngestionHandlerWithProcessor(svc InvoiceProcessor) *IngestionHandler {
	return &IngestionHandler{
		svc:     svc,
		limiter: rate.NewLimiter(rate.Limit(10), 20),
	}
}

// RegisterRoutes attaches routes to the provided gin Engine.
func (h *IngestionHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Health)
	r.POST("/invoices", h.Upload)
	if h.pool != nil {
		r.POST("/invoices/async", h.UploadAsync)
		r.GET("/invoices/jobs/:id", h.GetJob)
	}
}

// Health returns a liveness response that includes MongoDB connectivity.
func (h *IngestionHandler) Health(c *gin.Context) {
	mongoStatus := "connected"
	httpStatus := http.StatusOK
	serviceStatus := "ok"

	if h.db != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := h.db.Client().Ping(ctx, nil); err != nil {
			mongoStatus = "disconnected"
			serviceStatus = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	c.JSON(httpStatus, gin.H{
		"status":  serviceStatus,
		"service": "ingestion",
		"mongo":   mongoStatus,
	})
}

// Upload accepts a multipart image upload, runs OCR, and returns the created invoice.
//
// POST /invoices
// Content-Type: multipart/form-data
// Field: "invoice" — the image file
func (h *IngestionHandler) Upload(c *gin.Context) {
	// Rate limiting check.
	if h.limiter != nil && !h.limiter.Allow() {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded, please slow down"})
		return
	}

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

// UploadAsync accepts a multipart image upload and enqueues it for background OCR processing.
//
// POST /invoices/async
// Content-Type: multipart/form-data
// Field: "invoice" — the image file
func (h *IngestionHandler) UploadAsync(c *gin.Context) {
	if !h.limiter.Allow() {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	file, header, err := c.Request.FormFile("invoice")
	if err != nil {
		if isMaxBytesError(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image exceeds 10 MB size limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'invoice' file field: " + err.Error()})
		return
	}
	defer file.Close()

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

	// Seek back to start and read all bytes (the pool owns the data after this handler returns).
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset file reader: " + seekErr.Error()})
		return
	}
	imageData, readAllErr := io.ReadAll(file)
	if readAllErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file: " + readAllErr.Error()})
		return
	}

	// Create job record.
	jobID := uuid.New().String()
	job := &model.Job{
		ID:     jobID,
		Status: model.JobStatusPending,
	}
	if createErr := h.jobRepo.Create(c.Request.Context(), job); createErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job: " + createErr.Error()})
		return
	}

	// Enqueue to worker pool.
	task := worker.Task{
		JobID:     jobID,
		ImageData: imageData,
		MimeType:  detected,
		Filename:  header.Filename,
	}
	if submitErr := h.pool.Submit(task); submitErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker pool is full, try again later"})
		return
	}

	c.JSON(http.StatusAccepted, asyncResponse{
		JobID:  jobID,
		Status: model.JobStatusPending,
	})
}

// GetJob returns the current status of an async job.
//
// GET /invoices/jobs/:id
func (h *IngestionHandler) GetJob(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing job id"})
		return
	}

	job, err := h.jobRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrJobNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
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
