package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// extractedJSON is the intermediate JSON structure returned by any LLM provider.
type extractedJSON struct {
	InvoiceNumber string         `json:"invoice_number"`
	Date          string         `json:"date"`
	DueDate       *string        `json:"due_date"`
	Vendor        vendorJSON     `json:"vendor"`
	Customer      customerJSON   `json:"customer"`
	Currency      string         `json:"currency"`
	LineItems     []lineItemJSON `json:"line_items"`
	Subtotal      float64        `json:"subtotal"`
	TaxRate       *float64       `json:"tax_rate"`
	TaxAmount     float64        `json:"tax_amount"`
	Total         float64        `json:"total"`
	Notes         *string        `json:"notes"`
	Confidence    string         `json:"confidence"`
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

// ParseInvoiceJSON strips optional markdown fences, unmarshals the JSON, and
// maps it to model.ExtractedInvoice. Used by all LLM provider implementations.
func ParseInvoiceJSON(rawText string) (*model.ExtractedInvoice, error) {
	cleaned := stripCodeFences(rawText)

	var extracted extractedJSON
	if err := json.Unmarshal([]byte(cleaned), &extracted); err != nil {
		return nil, fmt.Errorf("llm: unmarshal response: %w", err)
	}

	lineItems := make([]model.LineItem, 0, len(extracted.LineItems))
	for _, li := range extracted.LineItems {
		lineItems = append(lineItems, model.LineItem{
			Description: li.Description,
			Quantity:    li.Quantity,
			UnitPrice:   li.UnitPrice,
			Amount:      li.Total,
		})
	}

	customerName := ""
	if extracted.Customer.Name != nil {
		customerName = *extracted.Customer.Name
	}

	return &model.ExtractedInvoice{
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
	}, nil
}

// stripCodeFences removes optional markdown code fences (```json ... ```) from s.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		idx := strings.Index(s, "\n")
		if idx != -1 {
			s = s[idx+1:]
		}
	}
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
