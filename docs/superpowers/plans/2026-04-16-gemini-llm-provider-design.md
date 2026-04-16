# Gemini LLM Provider — Design Spec

**Date:** 2026-04-16
**Project:** involens (Invoice OCR & Management System)
**Status:** Approved

---

## Overview

Add Google Gemini as a switchable LLM provider for invoice data extraction. The existing `InvoiceExtractor` interface and factory pattern make this a purely additive change — no modifications to the service, handler, worker pool, or repository layers.

---

## Goals

- Implement `internal/llm/gemini/client.go` conforming to the `llm.InvoiceExtractor` interface
- Make the Gemini model configurable via `GEMINI_MODEL` env var (default: `gemini-2.0-flash`)
- Reuse the existing JSON parsing logic by moving it to the shared `internal/llm` package
- Register `"gemini"` in the provider factory so switching requires only env var changes

## Non-Goals

- Gemini File API (upload-then-reference) — not needed; base64 inline is sufficient
- OpenAI or any other provider — out of scope for this change
- Changes to the service, handler, worker pool, repository, or API layers

---

## Architecture

### Components Changed

| File | Change |
|---|---|
| `internal/llm/provider.go` | Add exported `ParseInvoiceJSON()` helper (moved from Claude package) |
| `internal/llm/gemini/client.go` | New — Gemini `Client` implementing `InvoiceExtractor` |
| `internal/llm/gemini/client_test.go` | New — unit tests |
| `internal/llm/claude/client.go` | Update to call `llm.ParseInvoiceJSON()` instead of local copy |
| `internal/config/config.go` | Add `GeminiAPIKey` and `GeminiModel` fields |
| `cmd/ingestion/main.go` | Register `"gemini"` in the factory map |
| `go.mod` / `go.sum` | Add `github.com/google/generative-ai-go` dependency |

### Data Flow (Gemini path)

```
POST /invoices → IngestionHandler → InvoiceService → gemini.Client.ExtractInvoice()
    → base64-encode image (raw bytes passed to SDK as InlineData)
    → genai.GenerativeModel.GenerateContent(prompt + image)
    → extract text from Candidates[0].Content.Parts[0]
    → llm.ParseInvoiceJSON(text)
    → *model.ExtractedInvoice
```

---

## Configuration

Two new env vars:

| Env Var | Config Field | Default | Required |
|---|---|---|---|
| `GEMINI_API_KEY` | `GeminiAPIKey` | `""` | Yes, when `LLM_PROVIDER=gemini` |
| `GEMINI_MODEL` | `GeminiModel` | `"gemini-2.0-flash"` | No |

To switch to Gemini:

```env
LLM_PROVIDER=gemini
GEMINI_API_KEY=your-key-here
# GEMINI_MODEL=gemini-2.5-pro  # optional override
```

`gemini.New(cfg)` returns an error immediately if `GeminiAPIKey` is empty, consistent with how the Claude client validates `ANTHROPIC_API_KEY`.

---

## Gemini Client Design

```go
// internal/llm/gemini/client.go
package gemini

type Client struct {
    model *genai.GenerativeModel
}

func New(cfg *config.Config) (*Client, error)
func (c *Client) Name() string  // returns "gemini"
func (c *Client) ExtractInvoice(ctx context.Context, imageData []byte, mimeType string) (*model.ExtractedInvoice, error)
```

### `ExtractInvoice` Steps

1. Build a text part from the system prompt
2. Build an `InlineData` blob from raw image bytes + mimeType (SDK handles encoding)
3. Call `model.GenerateContent(ctx, promptPart, imagePart)`
4. Extract text from `resp.Candidates[0].Content.Parts[0]`
5. Call `llm.ParseInvoiceJSON(text)` to produce `*model.ExtractedInvoice`

### Error Handling

| Condition | Error |
|---|---|
| Empty `GEMINI_API_KEY` | Returned from `New()`: `"gemini: GEMINI_API_KEY is required"` |
| No candidates in response | `"gemini: empty response from API"` |
| Empty parts in candidate | `"gemini: no text in response"` |
| JSON parse failure | `"gemini: unmarshal response: ..."` (via `llm.ParseInvoiceJSON`) |

---

## Shared `ParseInvoiceJSON` Helper

`parseInvoiceJSON` and `stripCodeFences` currently live in `internal/llm/claude/client.go`. They are provider-agnostic and will be:

- Moved to `internal/llm/provider.go` as exported `ParseInvoiceJSON` and unexported `stripCodeFences`
- The Claude client updated to call `llm.ParseInvoiceJSON(rawText)` — no behaviour change

---

## Testing

`internal/llm/gemini/client_test.go` covers:

- `New()` returns error when `GeminiAPIKey` is empty
- `Name()` returns `"gemini"`
- `ExtractInvoice()` with a mock HTTP server or SDK mock: valid JSON response → correct `*model.ExtractedInvoice`
- `ExtractInvoice()` with empty candidates → error
- `ExtractInvoice()` with malformed JSON → error

The mock LLM (`internal/llm/mock`) remains unchanged and continues to be used for integration tests.

---

## Factory Registration

`cmd/ingestion/main.go` `newExtractor()` updated:

```go
factories := map[string]llm.ExtractorFactory{
    "claude": func(c *config.Config) (llm.InvoiceExtractor, error) { return claude.New(c) },
    "gemini": func(c *config.Config) (llm.InvoiceExtractor, error) { return gemini.New(c) },
    "mock":   func(c *config.Config) (llm.InvoiceExtractor, error) { return mock.New(c) },
}
```

Error message updated to: `unknown LLM provider %q — supported: claude, gemini, mock`

---

## Dependency

Add to `go.mod`:

```
github.com/google/generative-ai-go v0.x.x
```

Run `go get github.com/google/generative-ai-go/genai` to fetch and pin the version.
