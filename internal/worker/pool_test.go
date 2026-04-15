package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/google/uuid"
	"github.com/involens/invoice-ocr/internal/model"
	"github.com/involens/invoice-ocr/internal/repository"
	"github.com/involens/invoice-ocr/internal/worker"
)

// fakeJobRepo is an in-memory JobRepo for testing.
type fakeJobRepo struct {
	mu   sync.Mutex
	jobs map[string]*model.Job
}

func newFakeJobRepo() *fakeJobRepo {
	return &fakeJobRepo{jobs: make(map[string]*model.Job)}
}

func (r *fakeJobRepo) Create(_ context.Context, job *model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	cp := *job
	r.jobs[job.ID] = &cp
	return nil
}

func (r *fakeJobRepo) GetByID(_ context.Context, id string) (*model.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return nil, repository.ErrJobNotFound
	}
	cp := *j
	return &cp, nil
}

func (r *fakeJobRepo) Update(_ context.Context, id string, update bson.M) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return repository.ErrJobNotFound
	}
	if s, ok := update["status"].(string); ok {
		j.Status = s
	}
	if inv, ok := update["invoice_id"].(string); ok {
		j.InvoiceID = inv
	}
	if errMsg, ok := update["error"].(string); ok {
		j.Error = errMsg
	}
	j.UpdatedAt = time.Now().UTC()
	return nil
}

// stubProcessorService returns a fixed result or error.
type stubProcessorService struct {
	result *model.Invoice
	err    error
}

func (s *stubProcessorService) ProcessInvoiceData(_ context.Context, _ []byte, _ string, _ string) (*model.Invoice, error) {
	return s.result, s.err
}

func TestPool_SuccessfulTask_JobBecomeDone(t *testing.T) {
	invoiceID := bson.NewObjectID()
	svc := &stubProcessorService{result: &model.Invoice{ID: invoiceID}}

	repo := newFakeJobRepo()
	pool := worker.NewPool(2, svc, repo)
	pool.Start(context.Background())

	jobID := uuid.New().String()
	if err := repo.Create(context.Background(), &model.Job{ID: jobID, Status: model.JobStatusPending}); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := pool.Submit(worker.Task{JobID: jobID, ImageData: []byte{0xFF, 0xD8}, MimeType: "image/jpeg", Filename: "test.jpg"}); err != nil {
		t.Fatalf("submit task: %v", err)
	}

	pool.Stop()

	got, err := repo.GetByID(context.Background(), jobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != model.JobStatusDone {
		t.Errorf("job.Status = %q, want %q", got.Status, model.JobStatusDone)
	}
	if got.InvoiceID != invoiceID.Hex() {
		t.Errorf("job.InvoiceID = %q, want %q", got.InvoiceID, invoiceID.Hex())
	}
	if got.Error != "" {
		t.Errorf("job.Error should be empty, got %q", got.Error)
	}
}

func TestPool_FailedTask_JobBecomeFailed(t *testing.T) {
	svc := &stubProcessorService{err: errors.New("extraction failed: bad image")}

	repo := newFakeJobRepo()
	pool := worker.NewPool(2, svc, repo)
	pool.Start(context.Background())

	jobID := uuid.New().String()
	if err := repo.Create(context.Background(), &model.Job{ID: jobID, Status: model.JobStatusPending}); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := pool.Submit(worker.Task{JobID: jobID, ImageData: []byte{0xFF, 0xD8}, MimeType: "image/jpeg", Filename: "test.jpg"}); err != nil {
		t.Fatalf("submit task: %v", err)
	}

	pool.Stop()

	got, err := repo.GetByID(context.Background(), jobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != model.JobStatusFailed {
		t.Errorf("job.Status = %q, want %q", got.Status, model.JobStatusFailed)
	}
	if got.Error == "" {
		t.Error("job.Error should not be empty on failure")
	}
	if got.InvoiceID != "" {
		t.Errorf("job.InvoiceID should be empty on failure, got %q", got.InvoiceID)
	}
}
