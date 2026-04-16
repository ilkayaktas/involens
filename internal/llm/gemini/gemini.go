// Package gemini implements the llm.InvoiceExtractor interface using the
// Google Gemini generative AI API.
package gemini

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/model"
)

// Extractor calls the Gemini API to extract structured invoice data from images.
type Extractor struct {
	client *genai.Client
	model  string
}

// New creates a new Gemini Extractor using the API key from cfg.
func New(cfg *config.Config) (*Extractor, error) {
	if cfg.GeminiAPIKey == "" {
		return nil, fmt.Errorf("gemini: GEMINI_API_KEY is required")
	}
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}
	modelName := cfg.GeminiModel
	if modelName == "" {
		modelName = "gemini-2.0-flash"
	}
	return &Extractor{client: client, model: modelName}, nil
}

// Name returns the provider identifier.
func (e *Extractor) Name() string { return "gemini" }

// ExtractInvoice sends imageData to Gemini and returns structured invoice data.
// Full implementation is in a subsequent commit; this stub satisfies the interface.
func (e *Extractor) ExtractInvoice(_ context.Context, _ []byte, _ string) (*model.ExtractedInvoice, error) {
	return nil, fmt.Errorf("gemini: ExtractInvoice not yet implemented")
}
