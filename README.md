# Involens — Invoice OCR & Management System

A two-service Go application that accepts invoice images, extracts structured data using an LLM vision API, and exposes the results through a REST API.

## Architecture

```
┌─────────────────────────────────────────┐     ┌──────────────────────────────────────┐
│  Ingestion Service  :8080               │     │  API Service  :8081                  │
│                                         │     │                                      │
│  POST /invoices          (sync)         │     │  GET /v1/invoices    (list + search) │
│  POST /invoices/async    (async)        │     │  GET /v1/invoices/:id                │
│  GET  /invoices/jobs/:id (job status)   │     │  GET /v1/invoices/stats              │
│  GET  /health                           │     │  GET /health                         │
└───────────────────┬─────────────────────┘     └──────────────┬───────────────────────┘
                    │                                           │
                    └──────────────┬────────────────────────────┘
                                   │
                          ┌────────▼────────┐
                          │    MongoDB       │
                          │  invoices        │
                          │  jobs            │
                          └─────────────────┘
```

## Quick Start

```bash
# Copy env template and add your Anthropic API key
cp .env.example .env
# ANTHROPIC_API_KEY=sk-ant-...

# Start MongoDB + both services
docker compose up -d
```

Services will be available at:
- Ingestion: http://localhost:8080
- API: http://localhost:8081

## LLM Provider Configuration

The LLM provider is fully configurable via the `LLM_PROVIDER` environment variable. No code changes needed to switch providers.

| Value | Provider | Notes |
|---|---|---|
| `claude` (default) | Anthropic Claude | Requires `ANTHROPIC_API_KEY` |
| `gemini` | Google Gemini | Requires `GEMINI_API_KEY`; model via `GEMINI_MODEL` (default: `gemini-2.0-flash`) |
| `mock` | In-memory mock | For local development and testing |

To add a new provider (OpenAI, Gemini, etc.) implement the `InvoiceExtractor` interface in `internal/llm/` and register it in the factory in `cmd/ingestion/main.go`.

```go
// internal/llm/provider.go
type InvoiceExtractor interface {
    ExtractInvoice(ctx context.Context, imageData []byte, mimeType string) (*model.ExtractedInvoice, error)
    Name() string
}
```

## API Reference

### Upload an invoice (synchronous)

```http
POST /invoices
Content-Type: multipart/form-data

invoice=@invoice.jpg
```

Returns `201 Created`:
```json
{
  "id": "663f1a2b3c4d5e6f7a8b9c0d",
  "invoice_number": "INV-2026-001",
  "status": "processed",
  "confidence": "high",
  "total": 1800.00,
  "currency": "TRY",
  "vendor": { "name": "Acme Corp" }
}
```

### Upload an invoice (asynchronous)

```http
POST /invoices/async
Content-Type: multipart/form-data

invoice=@invoice.jpg
```

Returns `202 Accepted` immediately:
```json
{ "job_id": "550e8400-e29b-41d4-a716-446655440000", "status": "pending" }
```

Poll for result:
```http
GET /invoices/jobs/550e8400-e29b-41d4-a716-446655440000
```

```json
{ "id": "550e...", "status": "done", "invoice_id": "663f..." }
```

### List and search invoices

```http
GET /v1/invoices?vendor=Acme&from=2026-01-01&to=2026-03-31&currency=TRY&sort=date&order=desc&limit=20
```

```json
{
  "data": [ ... ],
  "total": 142,
  "skip": 0,
  "limit": 20
}
```

**Filter parameters:** `vendor`, `from`, `to`, `min_amount`, `max_amount`, `status`, `currency`, `sort`, `order`, `skip`, `limit`

### Dashboard stats

```http
GET /v1/invoices/stats
```

```json
{
  "total_invoices": 1284,
  "total_amount": 2450000.00,
  "currency": "TRY",
  "by_vendor": [
    { "name": "Acme Corp", "count": 45, "total": 120000 }
  ],
  "by_month": [
    { "month": "2026-01", "count": 120, "total": 340000 }
  ]
}
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `LLM_PROVIDER` | `claude` | LLM backend: `claude` or `mock` |
| `ANTHROPIC_API_KEY` | — | Required when `LLM_PROVIDER=claude` |
| `CLAUDE_MODEL` | `claude-sonnet-4-6` | Anthropic model ID |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection string |
| `MONGO_DB` | `invoices` | Database name |
| `INGESTION_PORT` | `8080` | Ingestion service port |
| `API_PORT` | `8081` | API service port |
| `STORAGE_PATH` | `./storage` | Local directory for uploaded images |
| `WORKER_COUNT` | `4` | Async worker pool size |
| `RATE_LIMIT_RPS` | `10` | Upload rate limit (requests/sec) |
| `RATE_LIMIT_BURST` | `20` | Upload rate limit burst size |
| `CORS_ORIGIN` | `*` | Allowed CORS origin for API service |

## Project Structure

```
.
├── cmd/
│   ├── ingestion/main.go       # Ingestion service entrypoint
│   └── api/main.go             # API service entrypoint
├── internal/
│   ├── config/                 # Env-var configuration
│   ├── model/                  # Invoice and Job domain models
│   ├── llm/
│   │   ├── provider.go         # InvoiceExtractor interface
│   │   ├── claude/             # Anthropic Claude implementation
│   │   └── mock/               # In-memory mock for testing
│   ├── imageutil/              # Image resize (max 1568px, JPEG output)
│   ├── repository/             # MongoDB CRUD, search, aggregations
│   ├── service/                # Business logic, idempotency, cross-validation
│   ├── handler/                # HTTP handlers (Gin)
│   ├── worker/                 # Async worker pool
│   └── retry/                  # Exponential backoff with jitter
├── docker-compose.yml
├── Dockerfile.ingestion
├── Dockerfile.api
└── Makefile
```

## Development

```bash
# Run tests
make test

# Run ingestion service locally (requires MongoDB)
make run-ingestion

# Run API service locally
make run-api

# Build binaries
make build
```

## Features

- **OCR extraction** — single-step image-to-JSON via Claude Vision API; handles Turkish invoice formats (KDV, fatura)
- **Idempotency** — SHA-256 image hash prevents duplicate processing under concurrent load
- **Async processing** — worker pool with job tracking; returns job ID immediately
- **Cross-validation** — line item totals verified against subtotal and tax; confidence auto-degraded on mismatch
- **Retry logic** — exponential backoff with jitter on transient LLM errors (429/5xx)
- **Rate limiting** — token bucket on upload endpoints (configurable RPS + burst)
- **Search & filter** — vendor, date range, amount range, status, currency, sorting
- **Dashboard stats** — aggregated totals by vendor and month
- **CORS** — configurable origin header for frontend consumption
- **Graceful shutdown** — SIGTERM drains in-flight requests and worker queue before exit
- **Health checks** — MongoDB connectivity probe on both services
