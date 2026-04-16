package claude

import (
	"context"
	"encoding/base64"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/llm"
	"github.com/involens/invoice-ocr/internal/model"
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

// Client implements llm.InvoiceExtractor using Anthropic Claude.
type Client struct {
	client anthropic.Client
	model  string
}

// New creates a new Claude client from the provided configuration.
func New(cfg *config.Config) (*Client, error) {
	if cfg.AnthropicAPIKey == "" {
		return nil, fmt.Errorf("claude: ANTHROPIC_API_KEY is required")
	}

	c := anthropic.NewClient(
		option.WithAPIKey(cfg.AnthropicAPIKey),
	)

	return &Client{
		client: c,
		model:  cfg.ClaudeModel,
	}, nil
}

// Name returns the provider identifier.
func (c *Client) Name() string {
	return "claude"
}

// ExtractInvoice sends the invoice image to Claude and parses the structured response.
func (c *Client) ExtractInvoice(ctx context.Context, imageData []byte, mimeType string) (*model.ExtractedInvoice, error) {
	// Base64-encode the image bytes.
	encoded := base64.StdEncoding.EncodeToString(imageData)

	// Build and send the Anthropic messages request.
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(mimeType, encoded),
				anthropic.NewTextBlock("Extract all invoice data from this image."),
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("claude: messages.new: %w", err)
	}

	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("claude: empty response from API")
	}

	// Extract text from first content block.
	rawText := msg.Content[0].Text

	// Parse the JSON response.
	return llm.ParseInvoiceJSON(rawText)
}
