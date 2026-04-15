package mock_test

import (
	"context"
	"testing"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/llm/mock"
	"github.com/involens/invoice-ocr/internal/model"
)

func TestMockExtractor_Name(t *testing.T) {
	c, err := mock.New(&config.Config{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := c.Name(); got != "mock" {
		t.Errorf("Name() = %q, want %q", got, "mock")
	}
}

func TestMockExtractor_ExtractInvoice(t *testing.T) {
	c, _ := mock.New(&config.Config{})
	inv, err := c.ExtractInvoice(context.Background(), []byte("fake"), "image/jpeg")
	if err != nil {
		t.Fatalf("ExtractInvoice() error: %v", err)
	}
	if inv == nil {
		t.Fatal("ExtractInvoice() returned nil")
	}
	if inv.InvoiceNumber == "" {
		t.Error("ExtractInvoice() returned empty InvoiceNumber")
	}
	if inv.Confidence != model.ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q", inv.Confidence, model.ConfidenceHigh)
	}
	if inv.Total <= 0 {
		t.Errorf("Total = %f, want > 0", inv.Total)
	}
}
