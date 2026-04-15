package service

import (
	"context"
	"testing"

	"github.com/involens/invoice-ocr/internal/model"
)

// TestIdempotency_SameImageReturnsCachedInvoice verifies that uploading the same
// image twice returns the existing invoice on the second call without inserting
// a new document into MongoDB.
func TestIdempotency_SameImageReturnsCachedInvoice(t *testing.T) {
	taxRate := 0.18
	result := &model.ExtractedInvoice{
		InvoiceNumber: "INV-IDEM-001",
		Vendor:        model.Vendor{Name: "IdempotentCorp"},
		Customer:      model.Customer{Name: "TestCustomer"},
		Date:          "2026-01-15",
		Currency:      "USD",
		LineItems: []model.LineItem{
			{Description: "Service", Quantity: 1, UnitPrice: 100, Amount: 100},
		},
		Subtotal:    100,
		TaxRate:     &taxRate,
		TaxAmount:   18,
		Total:       118,
		Confidence:  model.ConfidenceHigh,
		RawResponse: `{}`,
	}

	extractor := &countingExtractor{result: result}
	repo := &stubRepo{}
	svc := New(repo, extractor, t.TempDir())

	jpegData := makeTestJPEG()

	// First call — should process normally.
	file1, header1 := makeMultipartFile("invoice.jpg", jpegData)
	defer file1.Close()

	inv1, err := svc.ProcessInvoice(context.Background(), file1, header1)
	if err != nil {
		t.Fatalf("first ProcessInvoice() error: %v", err)
	}
	if inv1 == nil {
		t.Fatal("first ProcessInvoice() returned nil")
	}
	if inv1.InvoiceNumber != "INV-IDEM-001" {
		t.Errorf("first call InvoiceNumber = %q, want INV-IDEM-001", inv1.InvoiceNumber)
	}

	extractorCallsAfterFirst := extractor.calls
	if extractorCallsAfterFirst != 1 {
		t.Errorf("extractor.calls after first upload = %d, want 1", extractorCallsAfterFirst)
	}
	invoicesAfterFirst := len(repo.created)
	if invoicesAfterFirst != 1 {
		t.Errorf("repo.created after first upload = %d, want 1", invoicesAfterFirst)
	}

	// Second call with the exact same image bytes — must not call extractor or insert a new record.
	file2, header2 := makeMultipartFile("invoice.jpg", jpegData)
	defer file2.Close()

	inv2, err := svc.ProcessInvoice(context.Background(), file2, header2)
	if err != nil {
		t.Fatalf("second ProcessInvoice() error: %v", err)
	}
	if inv2 == nil {
		t.Fatal("second ProcessInvoice() returned nil")
	}

	// The returned invoice must be the same document as the first one.
	if inv2.ID != inv1.ID {
		t.Errorf("second call returned different invoice ID: got %s, want %s", inv2.ID.Hex(), inv1.ID.Hex())
	}

	// Extractor must NOT have been called again.
	if extractor.calls != extractorCallsAfterFirst {
		t.Errorf("extractor.calls after second upload = %d, want %d (idempotency should skip extraction)",
			extractor.calls, extractorCallsAfterFirst)
	}

	// No new document should have been inserted.
	if len(repo.created) != invoicesAfterFirst {
		t.Errorf("repo.created after second upload = %d, want %d (idempotency should skip insert)",
			len(repo.created), invoicesAfterFirst)
	}
}
