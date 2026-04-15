package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/repository"
)

// --- stub repository ---

type stubRepo struct {
	created []*model.Invoice
}

func (r *stubRepo) Create(_ context.Context, inv *model.Invoice) (*model.Invoice, error) {
	inv.ID = bson.NewObjectID()
	r.created = append(r.created, inv)
	return inv, nil
}

func (r *stubRepo) GetByID(_ context.Context, id bson.ObjectID) (*model.Invoice, error) {
	for _, inv := range r.created {
		if inv.ID == id {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id.Hex())
}

func (r *stubRepo) List(_ context.Context, _, _ int64) ([]*model.Invoice, error) {
	return r.created, nil
}

func (r *stubRepo) Search(_ context.Context, _ repository.SearchParams) ([]*model.Invoice, int64, error) {
	return r.created, int64(len(r.created)), nil
}

func (r *stubRepo) GetStats(_ context.Context) (*repository.Stats, error) {
	return &repository.Stats{
		TotalInvoices: int64(len(r.created)),
		ByVendor:      []repository.VendorStat{},
		ByMonth:       []repository.MonthlyStat{},
	}, nil
}

// --- stub extractor ---

// countingExtractor fails the first `failN` calls with `failErr`, then succeeds.
type countingExtractor struct {
	calls   int
	failN   int
	failErr error
	result  *model.ExtractedInvoice
}

func (e *countingExtractor) Name() string { return "counting-mock" }

func (e *countingExtractor) ExtractInvoice(_ context.Context, _ []byte, _ string) (*model.ExtractedInvoice, error) {
	e.calls++
	if e.calls <= e.failN {
		return nil, e.failErr
	}
	return e.result, nil
}

// --- helpers ---

// makeTestJPEG returns a valid 1×1 JPEG image as bytes.
func makeTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// makeMultipartFile wraps imageData in a minimal multipart.File + FileHeader.
func makeMultipartFile(filename string, data []byte) (multipart.File, *multipart.FileHeader) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("invoice", filename)
	_, _ = part.Write(data)
	_ = writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(int64(len(data)) + 1024)
	fileHeaders := form.File["invoice"]
	if len(fileHeaders) == 0 {
		panic("makeMultipartFile: no file header found")
	}
	fh := fileHeaders[0]
	f, _ := fh.Open()
	return f, fh
}

// fakeFileHeader creates a *multipart.FileHeader with a known Size.
func fakeFileHeader(filename string, size int64) *multipart.FileHeader {
	return &multipart.FileHeader{
		Filename: filename,
		Header:   make(textproto.MIMEHeader),
		Size:     size,
	}
}

// --- tests ---

// TestCrossValidate_LineItemMismatch verifies that a mismatch between line-item
// sum and subtotal causes confidence to degrade to "low" and status to "review".
func TestCrossValidate_LineItemMismatch(t *testing.T) {
	inv := &model.Invoice{
		InvoiceNumber: "INV-001",
		Confidence:    model.ConfidenceHigh,
		Status:        model.StatusProcessed,
		LineItems: []model.LineItem{
			{Description: "Widget", Quantity: 2, UnitPrice: 50, Amount: 100},
			{Description: "Gadget", Quantity: 1, UnitPrice: 200, Amount: 200},
		},
		Subtotal:  400, // wrong: actual sum is 300
		TaxAmount: 40,
		Total:     440,
	}

	crossValidate(inv)

	if inv.Confidence != model.ConfidenceLow {
		t.Errorf("Confidence = %q, want %q", inv.Confidence, model.ConfidenceLow)
	}
	if inv.Status != model.StatusReview {
		t.Errorf("Status = %q, want %q", inv.Status, model.StatusReview)
	}
	if inv.Notes == nil || !strings.Contains(*inv.Notes, "line items sum") {
		t.Errorf("Notes should describe line-item discrepancy, got: %v", inv.Notes)
	}
}

// TestCrossValidate_TotalMismatch verifies that total ≠ subtotal+tax degrades confidence.
func TestCrossValidate_TotalMismatch(t *testing.T) {
	inv := &model.Invoice{
		InvoiceNumber: "INV-002",
		Confidence:    model.ConfidenceHigh,
		Status:        model.StatusProcessed,
		Subtotal:      1000,
		TaxAmount:     100,
		Total:         1200, // wrong: should be 1100
	}

	crossValidate(inv)

	if inv.Confidence != model.ConfidenceLow {
		t.Errorf("Confidence = %q, want %q", inv.Confidence, model.ConfidenceLow)
	}
	if inv.Status != model.StatusReview {
		t.Errorf("Status = %q, want %q", inv.Status, model.StatusReview)
	}
}

// TestCrossValidate_Valid verifies that a consistent invoice is not degraded.
func TestCrossValidate_Valid(t *testing.T) {
	inv := &model.Invoice{
		InvoiceNumber: "INV-003",
		Confidence:    model.ConfidenceHigh,
		Status:        model.StatusProcessed,
		LineItems: []model.LineItem{
			{Description: "Item A", Quantity: 2, UnitPrice: 50, Amount: 100},
		},
		Subtotal:  100,
		TaxAmount: 18,
		Total:     118,
	}

	crossValidate(inv)

	if inv.Confidence != model.ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q (should not degrade valid invoice)", inv.Confidence, model.ConfidenceHigh)
	}
	if inv.Status != model.StatusProcessed {
		t.Errorf("Status = %q, want %q", inv.Status, model.StatusProcessed)
	}
}

// TestRetry_FailTwiceThenSucceed verifies that a transient extractor failure is
// retried and the invoice is ultimately processed.
func TestRetry_FailTwiceThenSucceed(t *testing.T) {
	taxRate := 0.1
	notes := "test"
	successResult := &model.ExtractedInvoice{
		InvoiceNumber: "INV-RETRY-001",
		Vendor:        model.Vendor{Name: "RetryVendor"},
		Customer:      model.Customer{Name: "RetryCustomer"},
		Date:          "2026-01-01",
		Currency:      "USD",
		LineItems: []model.LineItem{
			{Description: "Service", Quantity: 1, UnitPrice: 500, Amount: 500},
		},
		Subtotal:    500,
		TaxRate:     &taxRate,
		TaxAmount:   50,
		Total:       550,
		Notes:       &notes,
		Confidence:  model.ConfidenceHigh,
		RawResponse: `{}`,
	}

	extractor := &countingExtractor{
		failN:   2,
		failErr: errors.New("transient: 503 service unavailable"),
		result:  successResult,
	}

	repo := &stubRepo{}
	svc := New(repo, extractor, t.TempDir())

	jpegData := makeTestJPEG()
	file, header := makeMultipartFile("invoice.jpg", jpegData)
	defer file.Close()

	inv, err := svc.ProcessInvoice(context.Background(), file, header)
	if err != nil {
		t.Fatalf("ProcessInvoice() error: %v", err)
	}

	if extractor.calls != 3 {
		t.Errorf("extractor.calls = %d, want 3 (2 failures + 1 success)", extractor.calls)
	}

	if inv == nil {
		t.Fatal("ProcessInvoice() returned nil invoice")
	}
	if inv.InvoiceNumber != "INV-RETRY-001" {
		t.Errorf("InvoiceNumber = %q, want %q", inv.InvoiceNumber, "INV-RETRY-001")
	}
	if inv.Status != model.StatusProcessed {
		t.Errorf("Status = %q, want %q", inv.Status, model.StatusProcessed)
	}
}

// TestRetry_PermanentFailure verifies that a non-transient error is not retried.
func TestRetry_PermanentFailure(t *testing.T) {
	extractor := &countingExtractor{
		failN:   10, // always fail
		failErr: errors.New("invalid image format"),
	}

	repo := &stubRepo{}
	svc := New(repo, extractor, t.TempDir())

	jpegData := makeTestJPEG()
	file, header := makeMultipartFile("invoice.jpg", jpegData)
	defer file.Close()

	_, err := svc.ProcessInvoice(context.Background(), file, header)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Non-transient: should fail on first attempt, no retries.
	if extractor.calls != 1 {
		t.Errorf("extractor.calls = %d, want 1 (no retries for permanent error)", extractor.calls)
	}

	var llmErr *ErrLLMExtraction
	if !errors.As(err, &llmErr) {
		t.Errorf("expected *ErrLLMExtraction, got %T: %v", err, err)
	}
}

// TestProcessInvoice_CrossValidationInPipeline verifies the full pipeline degrades
// confidence when the extractor returns inconsistent totals.
func TestProcessInvoice_CrossValidationInPipeline(t *testing.T) {
	taxRate := 0.1
	badResult := &model.ExtractedInvoice{
		InvoiceNumber: "INV-BAD-001",
		Vendor:        model.Vendor{Name: "BadVendor"},
		Customer:      model.Customer{Name: "BadCustomer"},
		Date:          "2026-01-01",
		Currency:      "USD",
		LineItems: []model.LineItem{
			{Description: "Item", Quantity: 1, UnitPrice: 100, Amount: 100},
		},
		Subtotal:    500, // mismatch: line items sum to 100
		TaxRate:     &taxRate,
		TaxAmount:   50,
		Total:       550,
		Confidence:  model.ConfidenceHigh,
		RawResponse: `{}`,
	}

	extractor := &countingExtractor{result: badResult}
	repo := &stubRepo{}
	svc := New(repo, extractor, t.TempDir())

	jpegData := makeTestJPEG()
	file, header := makeMultipartFile("invoice.jpg", jpegData)
	defer file.Close()

	inv, err := svc.ProcessInvoice(context.Background(), file, header)
	if err != nil {
		t.Fatalf("ProcessInvoice() unexpected error: %v", err)
	}

	if inv.Confidence != model.ConfidenceLow {
		t.Errorf("Confidence = %q, want %q", inv.Confidence, model.ConfidenceLow)
	}
	if inv.Status != model.StatusReview {
		t.Errorf("Status = %q, want %q", inv.Status, model.StatusReview)
	}
	if inv.Notes == nil {
		t.Error("Notes should not be nil after cross-validation failure")
	}
}
