# Gemini LLM Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Google Gemini as a switchable LLM provider for invoice data extraction, selectable via `LLM_PROVIDER=gemini`.

**Architecture:** Move the provider-agnostic `parseInvoiceJSON` / `stripCodeFences` helpers from the Claude package into the shared `llm` package, then implement a new `internal/llm/gemini` package that satisfies the existing `InvoiceExtractor` interface using the `github.com/google/generative-ai-go` SDK. Register the provider in the factory in `cmd/ingestion/main.go` — no other layers change.

**Tech Stack:** Go 1.25, `github.com/google/generative-ai-go/genai`, `google.golang.org/api/option`

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `internal/llm/provider.go` | Add exported `ParseInvoiceJSON` + unexported `stripCodeFences` helpers (moved from Claude) |
| Create | `internal/llm/parse_test.go` | Tests for `ParseInvoiceJSON` and `stripCodeFences` (moved from Claude test file) |
| Modify | `internal/llm/claude/client.go` | Remove local `parseInvoiceJSON`/`stripCodeFences`; call `llm.ParseInvoiceJSON` instead |
| Modify | `internal/llm/claude/client_test.go` | Remove parse/fence tests (now in `llm` package) |
| Modify | `internal/config/config.go` | Add `GeminiAPIKey` and `GeminiModel` fields |
| Modify | `internal/config/config_test.go` | Add tests for new config fields |
| Create | `internal/llm/gemini/client.go` | `Client` implementing `InvoiceExtractor` via Gemini SDK |
| Create | `internal/llm/gemini/client_test.go` | Unit tests for Gemini client |
| Modify | `cmd/ingestion/main.go` | Register `"gemini"` factory entry |

---

## Task 1: Add Gemini SDK dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Fetch the Gemini SDK**

```bash
cd /Users/ilkayaktas/HomeServer/involens
go get github.com/google/generative-ai-go/genai@latest
go get google.golang.org/api/option@latest
```

Expected output: lines like `go: added github.com/google/generative-ai-go ...`

- [ ] **Step 2: Tidy modules**

```bash
go mod tidy
```

Expected: no errors. `go.mod` now includes `github.com/google/generative-ai-go`.

- [ ] **Step 3: Verify build still passes**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "build: Add google/generative-ai-go SDK for Gemini provider"
```

---

## Task 2: Move `ParseInvoiceJSON` to shared `llm` package

The `parseInvoiceJSON` and `stripCodeFences` functions are provider-agnostic. Moving them to `internal/llm/provider.go` lets the Gemini client (and any future provider) reuse them without importing the Claude package.

**Files:**
- Modify: `internal/llm/provider.go`
- Create: `internal/llm/parse_test.go`
- Modify: `internal/llm/claude/client.go`
- Modify: `internal/llm/claude/client_test.go`

- [ ] **Step 1: Write failing tests in the `llm` package**

Create `internal/llm/parse_test.go`:

```go
package llm

import (
	"testing"
)

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
	inv, err := ParseInvoiceJSON(sampleInvoiceJSON)
	if err != nil {
		t.Fatalf("ParseInvoiceJSON returned error: %v", err)
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
	inv, err := ParseInvoiceJSON(fenced)
	if err != nil {
		t.Fatalf("ParseInvoiceJSON with code fence returned error: %v", err)
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

	inv, err := ParseInvoiceJSON(jsonWithNullCustomer)
	if err != nil {
		t.Fatalf("ParseInvoiceJSON returned error: %v", err)
	}
	if inv.Customer.Name != "" {
		t.Errorf("customer.name = %q, want empty string for null", inv.Customer.Name)
	}
	if inv.DueDate != nil {
		t.Errorf("due_date = %v, want nil", inv.DueDate)
	}
}

func TestParseInvoiceJSON_InvalidJSON(t *testing.T) {
	_, err := ParseInvoiceJSON("not valid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL (ParseInvoiceJSON not defined)**

```bash
go test ./internal/llm/ -v -run TestParseInvoiceJSON
```

Expected: `FAIL` — `undefined: ParseInvoiceJSON`

- [ ] **Step 3: Add `ParseInvoiceJSON`, `stripCodeFences`, and internal JSON types to `internal/llm/provider.go`**

Append the following to the **end** of `internal/llm/provider.go` (keep the existing interface and factory type at the top):

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/model"
)
```

> **Note:** `provider.go` already imports `context`, `config`, and `model` — add only the missing ones (`encoding/json`, `fmt`, `strings`).

Add these types and functions to the bottom of `internal/llm/provider.go`:

```go
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
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/llm/ -v -run "TestParseInvoiceJSON|TestStripCodeFences"
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Update `internal/llm/claude/client.go` to use `llm.ParseInvoiceJSON`**

In `internal/llm/claude/client.go`:

**Remove** the following local types and functions (lines ~60–227):
- `extractedJSON`, `vendorJSON`, `customerJSON`, `lineItemJSON` struct definitions
- `parseInvoiceJSON` function
- `stripCodeFences` function

**Also remove** the now-unused imports: `encoding/json`, `strings` (keep `context`, `encoding/base64`, `fmt`).

**Update** the `ExtractInvoice` method — replace the `parseInvoiceJSON(rawText)` call with `llm.ParseInvoiceJSON(rawText)`:

```go
// ExtractInvoice sends the invoice image to Claude and parses the structured response.
func (c *Client) ExtractInvoice(ctx context.Context, imageData []byte, mimeType string) (*model.ExtractedInvoice, error) {
	encoded := base64.StdEncoding.EncodeToString(imageData)

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

	rawText := msg.Content[0].Text
	return llm.ParseInvoiceJSON(rawText)
}
```

**Add** the `llm` import to the import block in `claude/client.go`:

```go
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
```

- [ ] **Step 6: Update `internal/llm/claude/client_test.go` — remove tests now covered by `llm` package**

Delete the three test functions that now live in `internal/llm/parse_test.go`:
- `TestParseInvoiceJSON`
- `TestStripCodeFences`
- `TestParseInvoiceJSONWithCodeFence`
- `TestParseInvoiceJSON_NullCustomerName`
- The `sampleInvoiceJSON` const

The file should be left with only the package declaration and imports (it may become empty — that is fine; delete the file entirely if no tests remain).

- [ ] **Step 7: Build and test**

```bash
go build ./...
go test ./internal/llm/... -v
```

Expected: all tests PASS, no compile errors.

- [ ] **Step 8: Commit**

```bash
git add internal/llm/provider.go internal/llm/parse_test.go internal/llm/claude/client.go internal/llm/claude/client_test.go
git commit -m "refactor(llm): Move ParseInvoiceJSON to shared llm package"
```

---

## Task 3: Add Gemini config fields

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config test**

Open `internal/config/config_test.go` and add:

```go
func TestConfig_GeminiDefaults(t *testing.T) {
	// Unset env vars to ensure defaults are used.
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GEMINI_MODEL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GeminiAPIKey != "" {
		t.Errorf("GeminiAPIKey = %q, want empty string", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "gemini-2.0-flash" {
		t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, "gemini-2.0-flash")
	}
}

func TestConfig_GeminiEnvOverride(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key-123")
	t.Setenv("GEMINI_MODEL", "gemini-2.5-pro")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GeminiAPIKey != "test-key-123" {
		t.Errorf("GeminiAPIKey = %q, want %q", cfg.GeminiAPIKey, "test-key-123")
	}
	if cfg.GeminiModel != "gemini-2.5-pro" {
		t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, "gemini-2.5-pro")
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/config/ -v -run "TestConfig_Gemini"
```

Expected: `FAIL` — `cfg.GeminiAPIKey undefined`

- [ ] **Step 3: Add fields to `internal/config/config.go`**

In the `Config` struct, after the `ClaudeModel` field, add:

```go
// Gemini
GeminiAPIKey string // GEMINI_API_KEY
GeminiModel  string // GEMINI_MODEL, default "gemini-2.0-flash"
```

In the `Load()` function, after the `ClaudeModel` line, add:

```go
GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
GeminiModel:  getEnv("GEMINI_MODEL", "gemini-2.0-flash"),
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/config/ -v -run "TestConfig_Gemini"
```

Expected: both tests PASS.

- [ ] **Step 5: Run full config test suite**

```bash
go test ./internal/config/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): Add GeminiAPIKey and GeminiModel config fields"
```

---

## Task 4: Implement Gemini client

**Files:**
- Create: `internal/llm/gemini/client.go`
- Create: `internal/llm/gemini/client_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/llm/gemini/client_test.go`:

```go
package gemini_test

import (
	"testing"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/llm/gemini"
)

func TestNew_EmptyAPIKey(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "",
		GeminiModel:  "gemini-2.0-flash",
	}
	_, err := gemini.New(cfg)
	if err == nil {
		t.Fatal("New() with empty API key should return error, got nil")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "fake-key-for-test",
		GeminiModel:  "gemini-2.0-flash",
	}
	c, err := gemini.New(cfg)
	if err != nil {
		t.Fatalf("New() with valid config returned error: %v", err)
	}
	if c == nil {
		t.Fatal("New() returned nil client")
	}
}

func TestClient_Name(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "fake-key-for-test",
		GeminiModel:  "gemini-2.0-flash",
	}
	c, err := gemini.New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := c.Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/llm/gemini/ -v
```

Expected: `FAIL` — package `gemini` does not exist yet.

- [ ] **Step 3: Create `internal/llm/gemini/client.go`**

```go
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

// New creates a new Gemini client from the provided configuration.
// Returns an error if GeminiAPIKey is empty.
func New(cfg *config.Config) (*Client, error) {
	if cfg.GeminiAPIKey == "" {
		return nil, fmt.Errorf("gemini: GEMINI_API_KEY is required")
	}

	ctx := context.Background()
	genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiAPIKey))
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

// ExtractInvoice sends the invoice image to Gemini and parses the structured response.
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

	rawText := string(text)
	return llm.ParseInvoiceJSON(rawText)
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/llm/gemini/ -v
```

Expected: `TestNew_EmptyAPIKey`, `TestNew_ValidConfig`, `TestClient_Name` all PASS.

> Note: `TestNew_ValidConfig` creates a real `genai.Client` but never makes a network call — the SDK only connects when `GenerateContent` is called. If the SDK validates the key format at construction time and fails with a fake key, change `"fake-key-for-test"` to a syntactically valid key format like `"AIzaSyFakeKeyForTestingOnly"`.

- [ ] **Step 5: Build everything**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/llm/gemini/client.go internal/llm/gemini/client_test.go
git commit -m "feat(llm): Add Gemini provider implementing InvoiceExtractor"
```

---

## Task 5: Register Gemini in the factory and update README

**Files:**
- Modify: `cmd/ingestion/main.go`
- Modify: `README.md`

- [ ] **Step 1: Register Gemini in the factory**

In `cmd/ingestion/main.go`, add the `gemini` import and update `newExtractor()`:

Add to the import block:
```go
"github.com/involens/invoice-ocr/internal/llm/gemini"
```

Replace the `factories` map in `newExtractor()`:

```go
func newExtractor(cfg *config.Config) (llm.InvoiceExtractor, error) {
	factories := map[string]llm.ExtractorFactory{
		"claude": func(c *config.Config) (llm.InvoiceExtractor, error) { return claude.New(c) },
		"gemini": func(c *config.Config) (llm.InvoiceExtractor, error) { return gemini.New(c) },
		"mock":   func(c *config.Config) (llm.InvoiceExtractor, error) { return mock.New(c) },
	}

	factory, ok := factories[cfg.LLMProvider]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider %q — supported: claude, gemini, mock", cfg.LLMProvider)
	}

	return factory(cfg)
}
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/ingestion/
```

Expected: no errors.

- [ ] **Step 3: Update the LLM Provider Configuration table in `README.md`**

Find the LLM Provider Configuration section and update the table to add the Gemini row:

```markdown
| Value | Provider | Notes |
|---|---|---|
| `claude` (default) | Anthropic Claude | Requires `ANTHROPIC_API_KEY` |
| `gemini` | Google Gemini | Requires `GEMINI_API_KEY`; model via `GEMINI_MODEL` (default: `gemini-2.0-flash`) |
| `mock` | In-memory mock | For local development and testing |
```

- [ ] **Step 4: Run full test suite**

```bash
go test ./... 
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/ingestion/main.go README.md
git commit -m "feat(handler): Register Gemini provider in extractor factory"
```

---

## Task 6: Final verification

- [ ] **Step 1: Format check**

```bash
gofmt -l .
```

Expected: no output (no files need formatting). If any files are listed, run `gofmt -w .` and re-commit.

- [ ] **Step 2: Full build and test**

```bash
go build ./...
go test ./... -count=1
```

Expected: all packages build and all tests pass.

- [ ] **Step 3: Smoke test with mock provider (no real API key needed)**

```bash
LLM_PROVIDER=mock go run ./cmd/ingestion/ &
sleep 1
curl -s http://localhost:8080/health
kill %1
```

Expected: `{"status":"ok",...}` response.

- [ ] **Step 4: Verify Gemini is selectable (constructor-level check, no network)**

```go
// Quick manual check — add temporarily to main_test or run as a scratch program:
cfg := &config.Config{GeminiAPIKey: "", GeminiModel: "gemini-2.0-flash"}
_, err := gemini.New(cfg)
// err should be: "gemini: GEMINI_API_KEY is required"
fmt.Println(err)
```

Or validate via env:
```bash
LLM_PROVIDER=gemini GEMINI_API_KEY="" go run ./cmd/ingestion/ 2>&1 | head -5
```

Expected: `ingestion: init extractor: gemini: GEMINI_API_KEY is required` then process exits.

- [ ] **Step 5: Done ✓**

The Gemini provider is now fully registered. To use it in production:
```env
LLM_PROVIDER=gemini
GEMINI_API_KEY=your-real-gemini-api-key
GEMINI_MODEL=gemini-2.0-flash   # optional
```
