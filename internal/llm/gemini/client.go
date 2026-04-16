package gemini

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/llm"
	"github.com/involens/invoice-ocr/internal/model"
	"google.golang.org/api/option"
)

const systemPrompt = `You are an invoice data extraction system.
Extract ALL information from this invoice image.
Return ONLY valid JSON with no other text.

Required JSON schema:
{
  "invoice_number": "string",
  "date": "YYYY-MM-DD",
  "due_date": "YYYY-MM-DD or null",
  "vendor": {
    "name": "string",
    "tax_id": "string or null",
    "address": "string or null"
  },
  "customer": {
    "name": "string or null",
    "tax_id": "string or null",
    "address": "string or null"
  },
  "currency": "ISO 4217 code",
  "line_items": [
    {
      "description": "string",
      "quantity": number,
      "unit_price": number,
      "total": number
    }
  ],
  "subtotal": number,
  "tax_rate": number or null,
  "tax_amount": number,
  "total": number,
  "notes": "string or null",
  "confidence": "high" or "medium" or "low"
}

Rules:
- All monetary values as numbers, not strings
- Dates in YYYY-MM-DD format
- If a field is unreadable, set to null
- Set confidence based on image clarity and completeness
- Handle Turkish invoice formats (KDV = tax, fatura = invoice)`

// Client implements llm.InvoiceExtractor using Google Gemini.
type Client struct {
	model *genai.GenerativeModel
}

// New creates a new Gemini Client from the provided configuration.
// Returns an error if GeminiAPIKey is empty.
func New(cfg *config.Config) (*Client, error) {
	if cfg.GeminiAPIKey == "" {
		return nil, fmt.Errorf("gemini: GEMINI_API_KEY is required")
	}

	genaiClient, err := genai.NewClient(context.Background(), option.WithAPIKey(cfg.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}

	m := genaiClient.GenerativeModel(cfg.GeminiModel)
	return &Client{model: m}, nil
}

// Name returns the provider identifier.
func (c *Client) Name() string {
	return "gemini"
}

// ExtractInvoice sends the invoice image to Gemini and returns structured invoice data.
func (c *Client) ExtractInvoice(ctx context.Context, imageData []byte, mimeType string) (*model.ExtractedInvoice, error) {
	promptPart := genai.Text(systemPrompt)
	imagePart := genai.ImageData(mimeType, imageData)

	resp, err := c.model.GenerateContent(ctx, promptPart, imagePart)
	if err != nil {
		return nil, fmt.Errorf("gemini: generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: empty response from API")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini: no content in response candidate")
	}

	text, ok := candidate.Content.Parts[0].(genai.Text)
	if !ok {
		return nil, fmt.Errorf("gemini: unexpected part type in response")
	}

	return llm.ParseInvoiceJSON(string(text))
}
