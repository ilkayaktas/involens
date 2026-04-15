package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Confidence levels for OCR extraction quality.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Invoice processing statuses.
const (
	StatusProcessed = "processed"
	StatusFailed    = "failed"
	StatusPending   = "pending"
	StatusReview    = "review"
)

// Vendor holds vendor (seller) information extracted from the invoice.
type Vendor struct {
	Name    string  `bson:"name" json:"name"`
	Address *string `bson:"address,omitempty" json:"address,omitempty"`
	Email   *string `bson:"email,omitempty" json:"email,omitempty"`
	Phone   *string `bson:"phone,omitempty" json:"phone,omitempty"`
	TaxID   *string `bson:"tax_id,omitempty" json:"tax_id,omitempty"`
}

// Customer holds customer (buyer) information extracted from the invoice.
type Customer struct {
	Name    string  `bson:"name" json:"name"`
	Address *string `bson:"address,omitempty" json:"address,omitempty"`
	Email   *string `bson:"email,omitempty" json:"email,omitempty"`
	Phone   *string `bson:"phone,omitempty" json:"phone,omitempty"`
}

// LineItem represents a single line on an invoice.
type LineItem struct {
	Description string   `bson:"description" json:"description"`
	Quantity    float64  `bson:"quantity" json:"quantity"`
	UnitPrice   float64  `bson:"unit_price" json:"unit_price"`
	Amount      float64  `bson:"amount" json:"amount"`
	Unit        *string  `bson:"unit,omitempty" json:"unit,omitempty"`
	TaxRate     *float64 `bson:"tax_rate,omitempty" json:"tax_rate,omitempty"`
}

// Invoice is the primary MongoDB document / domain model for an invoice.
type Invoice struct {
	ID            bson.ObjectID `bson:"_id,omitempty" json:"id"`
	InvoiceNumber string        `bson:"invoice_number" json:"invoice_number"`
	Vendor        Vendor        `bson:"vendor" json:"vendor"`
	Customer      Customer      `bson:"customer" json:"customer"`
	Date          string        `bson:"date" json:"date"`
	DueDate       *string       `bson:"due_date,omitempty" json:"due_date,omitempty"`
	Currency      string        `bson:"currency" json:"currency"`
	LineItems     []LineItem    `bson:"line_items" json:"line_items"`
	Subtotal      float64       `bson:"subtotal" json:"subtotal"`
	TaxRate       *float64      `bson:"tax_rate,omitempty" json:"tax_rate,omitempty"`
	TaxAmount     float64       `bson:"tax_amount" json:"tax_amount"`
	Total         float64       `bson:"total" json:"total"`
	Notes         *string       `bson:"notes,omitempty" json:"notes,omitempty"`
	RawResponse   string        `bson:"raw_response" json:"raw_response"`
	ImagePath     string        `bson:"image_path" json:"image_path"`
	Confidence    string        `bson:"confidence" json:"confidence"` // "high" | "medium" | "low"
	Status        string        `bson:"status" json:"status"`          // "processed" | "failed" | "pending" | "review"
	CreatedAt     time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time     `bson:"updated_at" json:"updated_at"`
}

// ExtractedInvoice is the intermediate representation returned by an LLM provider
// before the data is persisted to MongoDB.
type ExtractedInvoice struct {
	InvoiceNumber string     `json:"invoice_number"`
	Vendor        Vendor     `json:"vendor"`
	Customer      Customer   `json:"customer"`
	Date          string     `json:"date"`
	DueDate       *string    `json:"due_date,omitempty"`
	Currency      string     `json:"currency"`
	LineItems     []LineItem `json:"line_items"`
	Subtotal      float64    `json:"subtotal"`
	TaxRate       *float64   `json:"tax_rate,omitempty"`
	TaxAmount     float64    `json:"tax_amount"`
	Total         float64    `json:"total"`
	Notes         *string    `json:"notes,omitempty"`
	Confidence    string     `json:"confidence"`
	RawResponse   string     `json:"raw_response"`
}
