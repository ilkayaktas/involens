package mock

import (
	"context"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/model"
)

// Client is a mock InvoiceExtractor that returns a hardcoded invoice — useful in tests.
type Client struct{}

// New creates a new mock client. cfg is accepted for interface parity but unused.
func New(_ *config.Config) (*Client, error) {
	return &Client{}, nil
}

// Name returns the provider identifier.
func (c *Client) Name() string {
	return "mock"
}

// ExtractInvoice returns a hardcoded invoice regardless of the input image.
func (c *Client) ExtractInvoice(_ context.Context, _ []byte, _ string) (*model.ExtractedInvoice, error) {
	dueDate := "2024-02-01"
	taxRate := 0.1
	notes := "Mock invoice for testing"

	return &model.ExtractedInvoice{
		InvoiceNumber: "MOCK-INV-2024-001",
		Vendor: model.Vendor{
			Name: "Acme Corp",
		},
		Customer: model.Customer{
			Name: "Test Customer Inc.",
		},
		Date:     "2024-01-15",
		DueDate:  &dueDate,
		Currency: "USD",
		LineItems: []model.LineItem{
			{
				Description: "Professional Services",
				Quantity:    10,
				UnitPrice:   150.00,
				Amount:      1500.00,
			},
			{
				Description: "Software License",
				Quantity:    1,
				UnitPrice:   500.00,
				Amount:      500.00,
			},
		},
		Subtotal:    2000.00,
		TaxRate:     &taxRate,
		TaxAmount:   200.00,
		Total:       2200.00,
		Notes:       &notes,
		Confidence:  model.ConfidenceHigh,
		RawResponse: `{"mock": true, "invoice_number": "MOCK-INV-2024-001"}`,
	}, nil
}
