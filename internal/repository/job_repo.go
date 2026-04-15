package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/involens/invoice-ocr/internal/model"
)

// ErrJobNotFound is returned when a requested job document does not exist.
var ErrJobNotFound = errors.New("job not found")

const jobCollectionName = "jobs"

// JobRepository provides CRUD operations for async jobs backed by MongoDB.
type JobRepository struct {
	collection *mongo.Collection
}

// NewJobRepository creates a JobRepository backed by the "jobs" collection.
func NewJobRepository(db *mongo.Database) (*JobRepository, error) {
	return &JobRepository{
		collection: db.Collection(jobCollectionName),
	}, nil
}

// Create inserts a new job document.
func (r *JobRepository) Create(ctx context.Context, job *model.Job) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now

	_, err := r.collection.InsertOne(ctx, job)
	if err != nil {
		return fmt.Errorf("job_repo: create: %w", err)
	}
	return nil
}

// GetByID retrieves a single job by its string ID.
// Returns ErrJobNotFound if the document does not exist.
func (r *JobRepository) GetByID(ctx context.Context, id string) (*model.Job, error) {
	var job model.Job
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("job_repo: get_by_id %s: %w", id, ErrJobNotFound)
		}
		return nil, fmt.Errorf("job_repo: get_by_id: %w", err)
	}
	return &job, nil
}

// Update performs a partial update on an existing job document.
func (r *JobRepository) Update(ctx context.Context, id string, update bson.M) error {
	update["updated_at"] = time.Now().UTC()

	result, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": update},
	)
	if err != nil {
		return fmt.Errorf("job_repo: update: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("job_repo: update %s: %w", id, ErrJobNotFound)
	}
	return nil
}
