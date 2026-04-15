package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/service"
)

// --- stub processor ---

type stubProcessor struct {
	inv *model.Invoice
	err error
}

func (s *stubProcessor) ProcessInvoice(_ context.Context, _ multipart.File, _ *multipart.FileHeader) (*model.Invoice, error) {
	return s.inv, s.err
}

// --- helpers ---

// makeJPEG returns a minimal valid JPEG (1x1 pixel).
func makeJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// buildMultipartBody creates a multipart/form-data body with a single "invoice" field.
func buildMultipartBody(filename string, content []byte) (body *bytes.Buffer, contentType string) {
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("invoice", filename)
	_, _ = part.Write(content)
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

func newTestEngine(proc InvoiceProcessor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newIngestionHandlerWithProcessor(proc)
	h.RegisterRoutes(r)
	return r
}

// --- tests ---

func TestUpload_FileTooLarge_Returns413(t *testing.T) {
	proc := &stubProcessor{
		inv: &model.Invoice{InvoiceNumber: "INV-001", Status: model.StatusProcessed, Confidence: model.ConfidenceHigh},
	}
	r := newTestEngine(proc)

	// Build a body that exceeds 10 MB via the multipart payload.
	// We fill with JPEG magic bytes at the front; the remainder can be zeros.
	oversized := make([]byte, maxUploadBytes+1024)
	jpegMagic := makeJPEG()
	copy(oversized, jpegMagic)

	body, ct := buildMultipartBody("big.jpg", oversized)

	req := httptest.NewRequest(http.MethodPost, "/invoices", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpload_UnsupportedFormat_Returns415(t *testing.T) {
	proc := &stubProcessor{}
	r := newTestEngine(proc)

	textContent := []byte("this is a plain text file, not an image")
	body, ct := buildMultipartBody("invoice.txt", textContent)

	req := httptest.NewRequest(http.MethodPost, "/invoices", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("want 415, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "unsupported image format") {
		t.Errorf("expected unsupported format error, got: %s", resp["error"])
	}
}

func TestUpload_ValidJPEG_Returns201(t *testing.T) {
	inv := &model.Invoice{
		InvoiceNumber: "INV-2026-001",
		Status:        model.StatusProcessed,
		Confidence:    model.ConfidenceHigh,
		Total:         1800.00,
		Currency:      "TRY",
		Vendor:        model.Vendor{Name: "Acme Corp"},
	}
	proc := &stubProcessor{inv: inv}
	r := newTestEngine(proc)

	jpegData := makeJPEG()
	body, ct := buildMultipartBody("invoice.jpg", jpegData)

	req := httptest.NewRequest(http.MethodPost, "/invoices", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp invoiceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.InvoiceNumber != "INV-2026-001" {
		t.Errorf("InvoiceNumber = %q, want %q", resp.InvoiceNumber, "INV-2026-001")
	}
	if resp.Status != model.StatusProcessed {
		t.Errorf("Status = %q, want %q", resp.Status, model.StatusProcessed)
	}
	if resp.Confidence != model.ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q", resp.Confidence, model.ConfidenceHigh)
	}
	if resp.Total != 1800.00 {
		t.Errorf("Total = %.2f, want 1800.00", resp.Total)
	}
	if resp.Vendor.Name != "Acme Corp" {
		t.Errorf("Vendor.Name = %q, want %q", resp.Vendor.Name, "Acme Corp")
	}
}

func TestUpload_LLMFailure_Returns422(t *testing.T) {
	llmErr := &service.ErrLLMExtraction{Cause: errors.New("rate limit exceeded")}
	proc := &stubProcessor{err: llmErr}
	r := newTestEngine(proc)

	// A valid JPEG so format validation passes.
	jpegData := makeJPEG()
	body, ct := buildMultipartBody("invoice.jpg", jpegData)

	req := httptest.NewRequest(http.MethodPost, "/invoices", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d; body: %s", w.Code, w.Body.String())
	}
}
