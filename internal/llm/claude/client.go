package claude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/involens/invoice-ocr/internal/config"
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

// extractedJSON is the intermediate JSON structure returned by Claude.
type extractedJSON struct {
	InvoiceNumber string          `json:"invoice_number"`
	Date          string          `json:"date"`
	DueDate       *string         `json:"due_date"`
	Vendor        vendorJSON      `json:"vendor"`
	Customer      customerJSON    `json:"customer"`
	Currency      string          `json:"currency"`
	LineItems     []lineItemJSON  `json:"line_items"`
	Subtotal      float64         `json:"subtotal"`
	TaxRate       *float64        `json:"tax_rate"`
	TaxAmount     float64         `json:"tax_amount"`
	Total         float64         `json:"total"`
	Notes         *string         `json:"notes"`
	Confidence    string          `json:"confidence"`
}

type vendorJSON struct {
	Name    string  `json:"name"`
	TaxID   *string `json:"tax_id"`
	Address *string `json:"address"`
}

type customerJSON struct {
	Name    *string `json:"name"`
	TaxID   *string `json:"tax_id"`
	Address *string `json:"address"`
}

type lineItemJSON struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Total       float64 `json:"total"`
}

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
	return parseInvoiceJSON(rawText)
}

// parseInvoiceJSON strips optional markdown fences, unmarshals the JSON, and
// maps it to model.ExtractedInvoice.
func parseInvoiceJSON(rawText string) (*model.ExtractedInvoice, error) {
	cleaned := stripCodeFences(rawText)

	var extracted extractedJSON
	if err := json.Unmarshal([]byte(cleaned), &extracted); err != nil {
		return nil, fmt.Errorf("claude: unmarshal response: %w", err)
	}

	// Map line items.
	lineItems := make([]model.LineItem, 0, len(extracted.LineItems))
	for _, li := range extracted.LineItems {
		lineItems = append(lineItems, model.LineItem{
			Description: li.Description,
			Quantity:    li.Quantity,
			UnitPrice:   li.UnitPrice,
			Amount:      li.Total,
		})
	}

	// Map customer name (nullable in JSON -> non-nullable in model).
	customerName := ""
	if extracted.Customer.Name != nil {
		customerName = *extracted.Customer.Name
	}

	invoice := &model.ExtractedInvoice{
		InvoiceNumber: extracted.InvoiceNumber,
		Vendor: model.Vendor{
			Name:    extracted.Vendor.Name,
			TaxID:   extracted.Vendor.TaxID,
			Address: extracted.Vendor.Address,
		},
		Customer: model.Customer{
			Name:    customerName,
			TaxID:   extracted.Customer.TaxID,
			Address: extracted.Customer.Address,
		},
		Date:        extracted.Date,
		DueDate:     extracted.DueDate,
		Currency:    extracted.Currency,
		LineItems:   lineItems,
		Subtotal:    extracted.Subtotal,
		TaxRate:     extracted.TaxRate,
		TaxAmount:   extracted.TaxAmount,
		Total:       extracted.Total,
		Notes:       extracted.Notes,
		Confidence:  extracted.Confidence,
		RawResponse: rawText,
	}

	return invoice, nil
}

// stripCodeFences removes optional markdown code fences (```json ... ```) from s.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	// Remove leading fence (```json or ```)
	if strings.HasPrefix(s, "```") {
		// Find the end of the first line.
		idx := strings.Index(s, "\n")
		if idx != -1 {
			s = s[idx+1:]
		}
	}
	// Remove trailing fence.
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
