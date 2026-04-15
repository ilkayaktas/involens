package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/involens/invoice-ocr/internal/model"
)

// connectTestDB connects to MongoDB using MONGO_URI env var and returns a temporary
// database. The returned cleanup function drops the database when called.
// If MONGO_URI is not set the test calling this function is skipped.
func connectTestDB(t *testing.T) (*InvoiceRepository, func()) {
	t.Helper()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("mongo ping: %v", err)
	}

	dbName := "involens_test_" + bson.NewObjectID().Hex()
	db := client.Database(dbName)

	repo, err := New(db)
	if err != nil {
		t.Fatalf("repository.New: %v", err)
	}

	cleanup := func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	}

	return repo, cleanup
}

// seedInvoices inserts invoices and returns them with IDs assigned.
func seedInvoices(t *testing.T, repo *InvoiceRepository, invoices []*model.Invoice) []*model.Invoice {
	t.Helper()
	var out []*model.Invoice
	for _, inv := range invoices {
		created, err := repo.Create(context.Background(), inv)
		if err != nil {
			t.Fatalf("seed Create: %v", err)
		}
		out = append(out, created)
	}
	return out
}

func TestSearch_NoFilters_ReturnsAll(t *testing.T) {
	repo, cleanup := connectTestDB(t)
	defer cleanup()

	seedInvoices(t, repo, []*model.Invoice{
		{
			InvoiceNumber: "INV-A1",
			Vendor:        model.Vendor{Name: "Alpha Corp"},
			Date:          "2026-01-15",
			Currency:      "USD",
			Total:         1000,
			Status:        model.StatusProcessed,
			Confidence:    model.ConfidenceHigh,
		},
		{
			InvoiceNumber: "INV-A2",
			Vendor:        model.Vendor{Name: "Beta Ltd"},
			Date:          "2026-02-20",
			Currency:      "EUR",
			Total:         2500,
			Status:        model.StatusProcessed,
			Confidence:    model.ConfidenceHigh,
		},
	})

	invoices, total, err := repo.Search(context.Background(), SearchParams{Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(invoices) != 2 {
		t.Errorf("len(invoices) = %d, want 2", len(invoices))
	}
}

func TestSearch_VendorFilter_ReturnsMatchingInvoices(t *testing.T) {
	repo, cleanup := connectTestDB(t)
	defer cleanup()

	seedInvoices(t, repo, []*model.Invoice{
		{
			InvoiceNumber: "INV-V1",
			Vendor:        model.Vendor{Name: "Acme Industries"},
			Date:          "2026-01-10",
			Currency:      "USD",
			Total:         500,
			Status:        model.StatusProcessed,
			Confidence:    model.ConfidenceHigh,
		},
		{
			InvoiceNumber: "INV-V2",
			Vendor:        model.Vendor{Name: "Global Tech"},
			Date:          "2026-01-12",
			Currency:      "USD",
			Total:         750,
			Status:        model.StatusProcessed,
			Confidence:    model.ConfidenceHigh,
		},
		{
			InvoiceNumber: "INV-V3",
			Vendor:        model.Vendor{Name: "ACME Solutions"},
			Date:          "2026-01-14",
			Currency:      "USD",
			Total:         1200,
			Status:        model.StatusProcessed,
			Confidence:    model.ConfidenceHigh,
		},
	})

	invoices, total, err := repo.Search(context.Background(), SearchParams{
		Vendor: "acme",
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("Search with vendor filter: %v", err)
	}

	// Case-insensitive regex should match "Acme Industries" and "ACME Solutions"
	if total != 2 {
		t.Errorf("total = %d, want 2 (both acme vendors)", total)
	}
	if len(invoices) != 2 {
		t.Errorf("len(invoices) = %d, want 2", len(invoices))
	}

	for _, inv := range invoices {
		if inv.Vendor.Name != "Acme Industries" && inv.Vendor.Name != "ACME Solutions" {
			t.Errorf("unexpected vendor: %q", inv.Vendor.Name)
		}
	}
}
