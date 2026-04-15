# Invoice OCR & Management System — Implementation Plan

**Approach:** Claude Vision API — Single-Step OCR + Extraction
**Stack:** Go + Anthropic API + MongoDB
**Date:** April 2026 | **Status:** Draft

---

## 1. Executive Summary

This document outlines the implementation plan for an Invoice OCR & Management System consisting of two microservices:

- **Ingestion Service:** Accepts invoice images, performs OCR and field extraction via Claude Vision API, and stores structured data in MongoDB.
- **API Service:** Reads MongoDB and exposes invoice data to a frontend via REST endpoints.

The chosen approach uses Claude's vision capability to perform OCR and structured data extraction in a single API call, eliminating the need for separate OCR engines or multi-stage pipelines.

---

## 2. Architecture Overview

### 2.1 System Components

```
Service 1: Invoice Ingestion Service (Go)
┌──────────────────────────────────────────────────┐
│  REST API (POST /invoices)                       │
│  ├─ Accept image (multipart/form-data)           │
│  ├─ Validate & resize image                      │
│  ├─ Call Claude Vision API                       │
│  ├─ Parse structured JSON response               │
│  ├─ Validate extracted data                      │
│  └─ Write to MongoDB                             │
└──────────────────────────────────────────────────┘

Service 2: Invoice API Service (Go)
┌──────────────────────────────────────────────────┐
│  REST API                                        │
│  ├─ GET /invoices         (list, paginated)      │
│  ├─ GET /invoices/:id     (single invoice)       │
│  ├─ GET /invoices/search  (filter/search)        │
│  └─ GET /invoices/stats   (dashboard stats)      │
└──────────────────────────────────────────────────┘

Shared: MongoDB (invoices collection)
```

### 2.2 Project Structure

```
invoice-ocr/
├── cmd/
│   ├── ingestion/main.go
│   └── api/main.go
├── internal/
│   ├── claude/client.go
│   ├── model/invoice.go
│   ├── repository/invoice_repo.go
│   ├── handler/
│   │   ├── ingestion_handler.go
│   │   └── api_handler.go
│   ├── service/invoice_service.go
│   └── config/config.go
├── docker-compose.yml
└── Makefile
```

---

## 3. LLM Provider Cost Comparison

All prices in USD as of April 2026. Identical assumptions used across all providers.

### 3.1 Assumptions

- Invoice image: ~1500×2000 px, resized to max 1568 px on longest side
- Image tokens: ~1,600 input tokens (formula: width × height / 750)
- System prompt + extraction instructions: ~300 input tokens
- **Total input per invoice: ~1,900 tokens**
- **Output (structured JSON): ~700 tokens**

### 3.2 Flagship Models

Best accuracy on complex, messy, or multilingual invoices.

| Model | Input $/1M | Output $/1M | Input Cost | Output Cost | **Per Invoice** |
|---|---|---|---|---|---|
| Claude Opus 4.6 | $5.00 | $25.00 | $0.0095 | $0.0175 | **$0.027** |
| GPT-5.4 | $2.50 | $15.00 | $0.0048 | $0.0105 | **$0.015** |
| Gemini 2.5 Pro | $1.25 | $10.00 | $0.0024 | $0.0070 | **$0.009** |
| GPT-4.1 | $2.00 | $8.00 | $0.0038 | $0.0056 | **$0.009** |

### 3.3 Balanced Models (Recommended)

Best value for most invoice OCR workloads.

| Model | Input $/1M | Output $/1M | Input Cost | Output Cost | **Per Invoice** |
|---|---|---|---|---|---|
| Claude Sonnet 4.6 | $3.00 | $15.00 | $0.0057 | $0.0105 | **$0.016** |
| GPT-4o | $2.50 | $10.00 | $0.0048 | $0.0070 | **$0.012** |
| Gemini 2.5 Flash | $0.30 | $2.50 | $0.0006 | $0.0018 | **$0.002** |

### 3.4 Budget Models

Lowest cost. Best for standardized formats with clean scans.

| Model | Input $/1M | Output $/1M | Input Cost | Output Cost | **Per Invoice** |
|---|---|---|---|---|---|
| Claude Haiku 4.5 | $1.00 | $5.00 | $0.0019 | $0.0035 | **$0.005** |
| GPT-5.4 Mini | $0.75 | $4.50 | $0.0014 | $0.0032 | **$0.005** |
| Gemini Flash-Lite | $0.10 | $0.40 | $0.0002 | $0.0003 | **$0.001** |

### 3.5 Monthly Cost Projection (30 Working Days)

| Model | 100/day | 1,000/day | 10,000/day | Batch 10K/day (50% off) |
|---|---|---|---|---|
| Claude Sonnet 4.6 | $48 | $480 | $4,800 | $2,400 |
| Claude Haiku 4.5 | $15 | $150 | $1,500 | $750 |
| GPT-5.4 | $45 | $450 | $4,500 | $2,250 |
| GPT-5.4 Mini | $15 | $150 | $1,500 | $750 |
| GPT-4o | $36 | $360 | $3,600 | $1,800 |
| GPT-4.1 | $27 | $270 | $2,700 | $1,350 |
| Gemini 2.5 Pro | $27 | $270 | $2,700 | $1,350 |
| Gemini 2.5 Flash | $6 | $60 | $600 | $300 |
| Gemini Flash-Lite | $3 | $30 | $300 | $150 |

### 3.6 Recommendation

- **Best accuracy:** Claude Sonnet 4.6 or GPT-5.4 (~$0.015–$0.016/invoice). Reliable on Turkish invoices, rotated images, varied layouts.
- **Best price/quality:** Gemini 2.5 Flash at $0.002/invoice. Handles most clean invoices well at 8× lower cost.
- **High volume (10K+/day):** Use a cascade — Gemini Flash first, escalate low-confidence results to Sonnet 4.6. Reduces average cost by 60–80%.
- **This plan uses Claude Sonnet 4.6** as the primary model given its strong structured output, Turkish language support, and your familiarity with the Anthropic API.

---

## 4. MongoDB Schema

### 4.1 Invoice Document

```json
{
  "_id": "ObjectId",
  "invoice_number": "INV-2026-001",
  "vendor": {
    "name": "Acme Corp",
    "tax_id": "1234567890",
    "address": "123 Main St, Istanbul"
  },
  "customer": {
    "name": "Your Company",
    "tax_id": "0987654321"
  },
  "date": "2026-04-10",
  "due_date": "2026-05-10",
  "currency": "TRY",
  "line_items": [
    {
      "description": "Widget A",
      "quantity": 10,
      "unit_price": 150.00,
      "total": 1500.00
    }
  ],
  "subtotal": 1500.00,
  "tax_rate": 20,
  "tax_amount": 300.00,
  "total": 1800.00,
  "raw_response": "{}",
  "image_path": "/storage/invoices/...",
  "confidence": "high",
  "status": "processed",
  "created_at": "ISODate",
  "updated_at": "ISODate"
}
```

### 4.2 Indexes

| Index | Type | Purpose |
|---|---|---|
| `{ invoice_number: 1 }` | Unique | Prevent duplicate invoices |
| `{ vendor.name: 1 }` | Standard | Vendor lookup and filtering |
| `{ date: -1 }` | Standard | Date sorting and range queries |
| `{ status: 1 }` | Standard | Filter by processing status |
| `{ created_at: -1 }` | Standard | Cursor-based pagination |

---

## 5. Claude Vision Integration

### 5.1 Extraction Prompt

```
You are an invoice data extraction system.
Extract ALL information from this invoice image.
Return ONLY valid JSON with no other text.

Required JSON schema:
{
  "invoice_number": string,
  "date": string (YYYY-MM-DD),
  "due_date": string | null,
  "vendor": {
    "name": string,
    "tax_id": string | null,
    "address": string | null
  },
  "customer": {
    "name": string | null,
    "tax_id": string | null,
    "address": string | null
  },
  "currency": string (ISO 4217),
  "line_items": [
    {
      "description": string,
      "quantity": number,
      "unit_price": number,
      "total": number
    }
  ],
  "subtotal": number,
  "tax_rate": number | null,
  "tax_amount": number,
  "total": number,
  "notes": string | null,
  "confidence": "high" | "medium" | "low"
}

Rules:
- All monetary values as numbers, not strings
- Dates in YYYY-MM-DD format
- If a field is unreadable, set to null
- Set confidence based on image clarity and completeness
- Handle Turkish invoice formats (KDV = tax, fatura = invoice)
```

### 5.2 API Call Flow

1. Resize image to max 1568 px on longest edge
2. Base64-encode the image bytes
3. POST to `api.anthropic.com/v1/messages` with image + system prompt
4. Parse JSON from `content[0].text`
5. Validate extracted data (e.g., `total = subtotal + tax_amount`)
6. Insert validated document into MongoDB

### 5.3 Image Preprocessing

- Resize to max 1568 px on longest dimension (Go: `disintegration/imaging`)
- Supported formats: JPEG, PNG, WebP, GIF
- Target file size: under 5 MB after resize
- Token formula: `(width × height) / 750`

---

## 6. Ingestion Service

### 6.1 Endpoint

```
POST /invoices
Content-Type: multipart/form-data
Body: image file (field name: 'invoice')

Response: 201 Created
{
  "id": "663f...",
  "invoice_number": "INV-2026-001",
  "status": "processed",
  "total": 1800.00
}
```

### 6.2 Processing Pipeline

1. Receive and validate image (format check, size < 10 MB)
2. Resize if dimensions exceed 1568 px
3. Store original to GCS bucket
4. Encode resized image to base64
5. Call Claude Vision API with extraction prompt
6. Parse and validate JSON response
7. Cross-check values (line item totals sum to subtotal)
8. Insert document into MongoDB with `status = "processed"`
9. Return extracted invoice summary

### 6.3 Error Handling

| Scenario | Action | Retry Strategy |
|---|---|---|
| Claude API 429 (rate limit) | Queue and retry | Exponential backoff with jitter |
| Claude API 500 | Retry request | Up to 3 attempts |
| Invalid JSON response | Retry with stricter prompt | 1 retry, then mark `failed` |
| Image too small/corrupt | Reject upload | No retry, return 400 |
| Confidence = low | Store with flag | Queue for manual review |

---

## 7. Query API Service

### 7.1 Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/invoices?page=1&size=20` | Paginated invoice list |
| GET | `/invoices/:id` | Single invoice by ID |
| GET | `/invoices/search` | Filter by vendor, date, amount, status |
| GET | `/invoices/stats` | Aggregated dashboard data |

### 7.2 Search & Filter Parameters

| Parameter | Type | Example |
|---|---|---|
| `vendor` | string | `?vendor=Acme` |
| `from` / `to` | date | `?from=2026-01-01&to=2026-03-31` |
| `min_amount` / `max_amount` | number | `?min_amount=1000&max_amount=5000` |
| `status` | enum | `?status=processed` |
| `currency` | string | `?currency=TRY` |
| `sort` | string | `?sort=date&order=desc` |

### 7.3 Stats Endpoint Response

```json
{
  "total_invoices": 1284,
  "total_amount": 2450000.00,
  "currency": "TRY",
  "by_vendor": [
    { "name": "Acme Corp", "count": 45, "total": 120000 },
    { "name": "TechSupply", "count": 32, "total": 89000 }
  ],
  "by_month": [
    { "month": "2026-01", "count": 120, "total": 340000 },
    { "month": "2026-02", "count": 145, "total": 410000 }
  ]
}
```

---

## 8. Async Processing & Batch Support

### 8.1 Async Upload Flow

- `POST /invoices` returns `202 Accepted` with a job ID immediately
- A worker pool (goroutines or SQS consumers) processes images in the background
- `GET /invoices/jobs/:id` returns processing status and result when complete

### 8.2 Anthropic Batch API (50% Savings)

- Collect invoice images into batches for non-urgent processing
- Submit as a single batch request to the Messages Batch API
- Results returned asynchronously within 24 hours
- Ideal for end-of-day or overnight processing workflows

### 8.3 Cascade Strategy (Cost Optimization)

Two-tier approach to minimize costs while maintaining accuracy:

1. Process all invoices first with a budget model (Haiku 4.5 or Gemini 2.5 Flash)
2. If extraction confidence is `"low"` or validation fails, re-process with Sonnet 4.6
3. This approach can reduce average per-invoice cost by 60–80%

---

## 9. Production Hardening

- **Idempotency:** SHA-256 image hash to detect and reject duplicate uploads
- **Rate limiting:** Token bucket on the upload endpoint to control throughput
- **Monitoring:** Track Claude API latency, error rates, and extraction confidence distribution
- **Image storage:** GCS bucket with lifecycle policies for archiving old images
- **Prompt caching:** System prompt is identical across all requests — 90% input cost reduction
- **Health checks:** `/health` endpoints for both services, integrated with K8s liveness/readiness probes
- **CORS:** Configure appropriate headers on the API service for frontend consumption
- **Deployment:** Docker containers on existing GKE cluster with Helm charts

---

## 10. Implementation Timeline

| Phase | Task | Duration | Deliverable |
|---|---|---|---|
| 1 | Project setup, Go modules, MongoDB schema, Docker Compose | 1 day | Running skeleton with MongoDB |
| 2 | Claude Vision API client, prompt engineering, response parsing | 2 days | Working extraction from test images |
| 3 | Ingestion service: upload endpoint, pipeline, error handling | 1–2 days | POST /invoices working end-to-end |
| 4 | Query API: CRUD, search, pagination, stats | 1–2 days | All GET endpoints functional |
| 5 | Async processing: worker pool, job tracking, batch API | 1 day | Async upload flow operational |
| 6 | Production: idempotency, monitoring, CORS, Docker, K8s | 1–2 days | Production-ready deployment |

**Total estimated timeline: 7–10 working days**

---

## 11. Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Claude API downtime | Processing blocked | Queue with retry; Gemini as failover provider |
| Poor extraction on unusual formats | Manual correction needed | Cascade strategy; prompt iteration |
| Cost overrun at high volume | Budget exceeded | Batch API + prompt caching + model cascade |
| Image quality issues | OCR failures | Validate image size/resolution on upload; return actionable errors |
| MongoDB write throughput | Bottleneck at scale | Write concern tuning; sharding if needed at 10K+/day |
