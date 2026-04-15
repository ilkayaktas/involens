package claude

import (
	"context"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/model"
)

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
//
// TODO(Phase 2): Implement the actual Anthropic Vision API call.
//   - Encode imageData as base64.
//   - Build a MessageNewParams with an image block + extraction prompt.
//   - Call c.client.Messages.New(ctx, params) with the configured model.
//   - Parse the JSON response into model.ExtractedInvoice.
func (c *Client) ExtractInvoice(_ context.Context, _ []byte, _ string) (*model.ExtractedInvoice, error) {
	// Stub: return a placeholder until Phase 2 implements the real API call.
	return &model.ExtractedInvoice{
		InvoiceNumber: "STUB-001",
		Vendor:        model.Vendor{Name: "Claude Stub Vendor"},
		Customer:      model.Customer{Name: "Claude Stub Customer"},
		Date:          "2024-01-01",
		Currency:      "USD",
		LineItems:     []model.LineItem{},
		Subtotal:      0,
		TaxAmount:     0,
		Total:         0,
		Confidence:    model.ConfidenceLow,
		RawResponse:   "stub — Phase 2 not yet implemented",
	}, nil
}
