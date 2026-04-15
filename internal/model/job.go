package model

import "time"

// Job processing statuses.
const (
	JobStatusPending    = "pending"
	JobStatusProcessing = "processing"
	JobStatusDone       = "done"
	JobStatusFailed     = "failed"
)

// Job represents an async invoice processing task.
type Job struct {
	ID        string    `bson:"_id" json:"id"`
	Status    string    `bson:"status" json:"status"`
	InvoiceID string    `bson:"invoice_id,omitempty" json:"invoice_id,omitempty"`
	Error     string    `bson:"error,omitempty" json:"error,omitempty"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
