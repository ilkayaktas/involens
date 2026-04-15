package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/repository"
	"github.com/involens/invoice-ocr/internal/service"
)

// --- stub repo for handler tests ---

type handlerStubRepo struct {
	invoices []*model.Invoice
}

func (r *handlerStubRepo) Create(_ context.Context, inv *model.Invoice) (*model.Invoice, error) {
	inv.ID = bson.NewObjectID()
	r.invoices = append(r.invoices, inv)
	return inv, nil
}

func (r *handlerStubRepo) GetByID(_ context.Context, id bson.ObjectID) (*model.Invoice, error) {
	for _, inv := range r.invoices {
		if inv.ID == id {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("invoice_repo: get_by_id %s: %w", id.Hex(), repository.ErrNotFound)
}

func (r *handlerStubRepo) List(_ context.Context, _, _ int64) ([]*model.Invoice, error) {
	return r.invoices, nil
}

func (r *handlerStubRepo) Search(_ context.Context, _ repository.SearchParams) ([]*model.Invoice, int64, error) {
	return r.invoices, int64(len(r.invoices)), nil
}

func (r *handlerStubRepo) GetByImageHash(_ context.Context, _ string) (*model.Invoice, error) {
	return nil, repository.ErrNotFound
}

func (r *handlerStubRepo) GetStats(_ context.Context) (*repository.Stats, error) {
	return &repository.Stats{
		TotalInvoices: int64(len(r.invoices)),
		TotalAmount:   0,
		Currency:      "USD",
		ByVendor:      []repository.VendorStat{},
		ByMonth:       []repository.MonthlyStat{},
	}, nil
}

// newTestRouter builds a gin engine backed by the given stub repo.
func newTestRouter(t *testing.T, repo *handlerStubRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := service.New(repo, nil, t.TempDir())
	h := NewAPIHandler(svc, nil)
	h.RegisterRoutes(r)
	return r
}

// --- tests ---

func TestListInvoices_Returns200WithDataArray(t *testing.T) {
	repo := &handlerStubRepo{
		invoices: []*model.Invoice{
			{
				ID:            bson.NewObjectID(),
				InvoiceNumber: "INV-001",
				Vendor:        model.Vendor{Name: "Acme"},
				Status:        model.StatusProcessed,
			},
			{
				ID:            bson.NewObjectID(),
				InvoiceNumber: "INV-002",
				Vendor:        model.Vendor{Name: "Globex"},
				Status:        model.StatusProcessed,
			},
		},
	}

	router := newTestRouter(t, repo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/invoices", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	data, ok := resp["data"]
	if !ok {
		t.Fatal("response missing 'data' key")
	}

	dataSlice, ok := data.([]interface{})
	if !ok {
		t.Fatalf("'data' is not an array, got %T", data)
	}

	if len(dataSlice) != 2 {
		t.Errorf("expected 2 items in data, got %d", len(dataSlice))
	}

	if _, ok := resp["total"]; !ok {
		t.Error("response missing 'total' key")
	}
}

func TestGetInvoice_ValidID_Returns200(t *testing.T) {
	id := bson.NewObjectID()
	repo := &handlerStubRepo{
		invoices: []*model.Invoice{
			{
				ID:            id,
				InvoiceNumber: "INV-100",
				Vendor:        model.Vendor{Name: "TestVendor"},
				Status:        model.StatusProcessed,
			},
		},
	}

	router := newTestRouter(t, repo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/invoices/"+id.Hex(), nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var inv model.Invoice
	if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil {
		t.Fatalf("unmarshal invoice: %v", err)
	}

	if inv.InvoiceNumber != "INV-100" {
		t.Errorf("InvoiceNumber = %q, want INV-100", inv.InvoiceNumber)
	}
}

func TestGetInvoice_UnknownID_Returns404(t *testing.T) {
	repo := &handlerStubRepo{}

	router := newTestRouter(t, repo)
	w := httptest.NewRecorder()
	// Use a valid ObjectID hex that does not exist in the stub.
	req, _ := http.NewRequest(http.MethodGet, "/v1/invoices/"+bson.NewObjectID().Hex(), nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStats_Returns200(t *testing.T) {
	repo := &handlerStubRepo{
		invoices: []*model.Invoice{
			{
				ID:            bson.NewObjectID(),
				InvoiceNumber: "INV-STATS-001",
				Vendor:        model.Vendor{Name: "StatVendor"},
				Total:         1000,
				Currency:      "USD",
				Status:        model.StatusProcessed,
			},
		},
	}

	router := newTestRouter(t, repo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/invoices/stats", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats repository.Stats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}

	if stats.TotalInvoices != 1 {
		t.Errorf("TotalInvoices = %d, want 1", stats.TotalInvoices)
	}
}
