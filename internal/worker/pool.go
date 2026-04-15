package worker

import (
	"context"
	"errors"
	"log"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/involens/invoice-ocr/internal/model"
)

// ErrPoolFull is returned by Submit when the task queue is at capacity.
var ErrPoolFull = errors.New("worker pool: queue is full")

// JobRepo is the persistence interface the pool needs for job state transitions.
type JobRepo interface {
	Create(ctx context.Context, job *model.Job) error
	GetByID(ctx context.Context, id string) (*model.Job, error)
	Update(ctx context.Context, id string, update bson.M) error
}

// Task holds everything a worker needs to process an async invoice job.
type Task struct {
	JobID     string
	ImageData []byte
	MimeType  string
	Filename  string
}

// ProcessorService is the interface the pool needs from the service layer.
type ProcessorService interface {
	ProcessInvoiceData(ctx context.Context, imageData []byte, mimeType string, filename string) (*model.Invoice, error)
}

// Pool is a fixed-size goroutine worker pool for async invoice processing.
type Pool struct {
	workers int
	queue   chan Task
	svc     ProcessorService
	jobRepo JobRepo
	wg      sync.WaitGroup
}

// NewPool creates a Pool with the given number of workers.
// The internal task queue has capacity workers*10.
func NewPool(workers int, svc ProcessorService, jobRepo JobRepo) *Pool {
	return &Pool{
		workers: workers,
		queue:   make(chan Task, workers*10),
		svc:     svc,
		jobRepo: jobRepo,
	}
}

// Start launches the worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	for range p.workers {
		p.wg.Add(1)
		go p.runWorker(ctx)
	}
}

// Submit enqueues a task. Returns ErrPoolFull if the queue is at capacity.
func (p *Pool) Submit(task Task) error {
	select {
	case p.queue <- task:
		return nil
	default:
		return ErrPoolFull
	}
}

// Stop closes the task queue and waits for all workers to drain and exit.
func (p *Pool) Stop() {
	close(p.queue)
	p.wg.Wait()
}

// runWorker is the main loop for a single worker goroutine.
func (p *Pool) runWorker(ctx context.Context) {
	defer p.wg.Done()
	for task := range p.queue {
		p.process(ctx, task)
	}
}

// process executes a single task: marks the job processing → calls service → marks done/failed.
func (p *Pool) process(ctx context.Context, task Task) {
	// Use background context so a cancelled ctx doesn't abort in-flight work.
	bgCtx := context.Background()
	_ = ctx

	// Mark as processing.
	if err := p.jobRepo.Update(bgCtx, task.JobID, bson.M{"status": model.JobStatusProcessing}); err != nil {
		log.Printf("worker: mark processing job %s: %v", task.JobID, err)
	}

	inv, err := p.svc.ProcessInvoiceData(bgCtx, task.ImageData, task.MimeType, task.Filename)
	if err != nil {
		if updateErr := p.jobRepo.Update(bgCtx, task.JobID, bson.M{
			"status": model.JobStatusFailed,
			"error":  err.Error(),
		}); updateErr != nil {
			log.Printf("worker: mark failed job %s: %v", task.JobID, updateErr)
		}
		return
	}

	if updateErr := p.jobRepo.Update(bgCtx, task.JobID, bson.M{
		"status":     model.JobStatusDone,
		"invoice_id": inv.ID.Hex(),
	}); updateErr != nil {
		log.Printf("worker: mark done job %s: %v", task.JobID, updateErr)
	}
}
