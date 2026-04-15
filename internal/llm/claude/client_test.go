package claude

import (
	"testing"
)

// sampleInvoiceJSON is a representative JSON response from the Claude API.
const sampleInvoiceJSON = `{
  "invoice_number": "INV-2024-001",
  "date": "2024-01-15",
  "due_date": "2024-02-15",
  "vendor": {
    "name": "Acme Corp",
    "tax_id": "TR123456789",
    "address": "123 Main St, Istanbul"
  },
  "customer": {
    "name": "Test Customer",
    "tax_id": null,
    "address": "456 Side St, Ankara"
  },
  "currency": "TRY",
  "line_items": [
    {
      "description": "Professional Services",
      "quantity": 10,
      "unit_price": 150.00,
      "total": 1500.00
    },
    {
      "description": "Software License",
      "quantity": 1,
      "unit_price": 500.00,
      "total": 500.00
    }
  ],
  "subtotal": 2000.00,
  "tax_rate": 0.18,
  "tax_amount": 360.00,
  "total": 2360.00,
  "notes": "KDV dahil",
  "confidence": "high"
}`

func TestParseInvoiceJSON(t *testing.T) {
	inv, err := parseInvoiceJSON(sampleInvoiceJSON)
	if err != nil {
		t.Fatalf("parseInvoiceJSON returned error: %v", err)
	}

	if inv.InvoiceNumber != "INV-2024-001" {
		t.Errorf("invoice_number = %q, want %q", inv.InvoiceNumber, "INV-2024-001")
	}
	if inv.Date != "2024-01-15" {
		t.Errorf("date = %q, want %q", inv.Date, "2024-01-15")
	}
	if inv.DueDate == nil || *inv.DueDate != "2024-02-15" {
		t.Errorf("due_date = %v, want %q", inv.DueDate, "2024-02-15")
	}
	if inv.Vendor.Name != "Acme Corp" {
		t.Errorf("vendor.name = %q, want %q", inv.Vendor.Name, "Acme Corp")
	}
	if inv.Vendor.TaxID == nil || *inv.Vendor.TaxID != "TR123456789" {
		t.Errorf("vendor.tax_id = %v, want %q", inv.Vendor.TaxID, "TR123456789")
	}
	if inv.Customer.Name != "Test Customer" {
		t.Errorf("customer.name = %q, want %q", inv.Customer.Name, "Test Customer")
	}
	if inv.Currency != "TRY" {
		t.Errorf("currency = %q, want %q", inv.Currency, "TRY")
	}
	if len(inv.LineItems) != 2 {
		t.Errorf("line_items count = %d, want 2", len(inv.LineItems))
	} else {
		if inv.LineItems[0].Description != "Professional Services" {
			t.Errorf("line_items[0].description = %q", inv.LineItems[0].Description)
		}
		if inv.LineItems[0].Amount != 1500.00 {
			t.Errorf("line_items[0].amount = %v, want 1500.00", inv.LineItems[0].Amount)
		}
	}
	if inv.Subtotal != 2000.00 {
		t.Errorf("subtotal = %v, want 2000.00", inv.Subtotal)
	}
	if inv.TaxRate == nil || *inv.TaxRate != 0.18 {
		t.Errorf("tax_rate = %v, want 0.18", inv.TaxRate)
	}
	if inv.TaxAmount != 360.00 {
		t.Errorf("tax_amount = %v, want 360.00", inv.TaxAmount)
	}
	if inv.Total != 2360.00 {
		t.Errorf("total = %v, want 2360.00", inv.Total)
	}
	if inv.Notes == nil || *inv.Notes != "KDV dahil" {
		t.Errorf("notes = %v, want %q", inv.Notes, "KDV dahil")
	}
	if inv.Confidence != "high" {
		t.Errorf("confidence = %q, want %q", inv.Confidence, "high")
	}
	if inv.RawResponse != sampleInvoiceJSON {
		t.Error("RawResponse should be the original text")
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no fences",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "json fence",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "plain fence",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "extra whitespace",
			input: "  ```json\n{\"key\": \"value\"}\n```  ",
			want:  `{"key": "value"}`,
		},
		{
			name:  "no trailing fence",
			input: "```json\n{\"key\": \"value\"}",
			want:  `{"key": "value"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCodeFences(tc.input)
			if got != tc.want {
				t.Errorf("stripCodeFences(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseInvoiceJSONWithCodeFence(t *testing.T) {
	fenced := "```json\n" + sampleInvoiceJSON + "\n```"
	inv, err := parseInvoiceJSON(fenced)
	if err != nil {
		t.Fatalf("parseInvoiceJSON with code fence returned error: %v", err)
	}
	if inv.InvoiceNumber != "INV-2024-001" {
		t.Errorf("invoice_number = %q, want %q", inv.InvoiceNumber, "INV-2024-001")
	}
}

func TestParseInvoiceJSON_NullCustomerName(t *testing.T) {
	jsonWithNullCustomer := `{
		"invoice_number": "INV-001",
		"date": "2024-01-01",
		"due_date": null,
		"vendor": {"name": "Vendor Inc", "tax_id": null, "address": null},
		"customer": {"name": null, "tax_id": null, "address": null},
		"currency": "USD",
		"line_items": [],
		"subtotal": 0,
		"tax_rate": null,
		"tax_amount": 0,
		"total": 0,
		"notes": null,
		"confidence": "low"
	}`

	inv, err := parseInvoiceJSON(jsonWithNullCustomer)
	if err != nil {
		t.Fatalf("parseInvoiceJSON returned error: %v", err)
	}
	if inv.Customer.Name != "" {
		t.Errorf("customer.name = %q, want empty string for null", inv.Customer.Name)
	}
	if inv.DueDate != nil {
		t.Errorf("due_date = %v, want nil", inv.DueDate)
	}
}
