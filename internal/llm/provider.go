package llm

import (
	"context"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/model"
)

// InvoiceExtractor is the provider-agnostic interface for LLM-based invoice OCR.
// Implementations can be swapped by changing LLM_PROVIDER in config.
type InvoiceExtractor interface {
	// ExtractInvoice sends the raw image bytes to the LLM and returns structured invoice data.
	ExtractInvoice(ctx context.Context, imageData []byte, mimeType string) (*model.ExtractedInvoice, error)

	// Name returns a human-readable identifier for this provider (e.g. "claude", "mock").
	Name() string
}

// ExtractorFactory is a constructor function that creates an InvoiceExtractor from config.
type ExtractorFactory func(cfg *config.Config) (InvoiceExtractor, error)
