package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/involens/invoice-ocr/internal/model"
)

const collectionName = "invoices"

// InvoiceRepository provides CRUD operations for invoices backed by MongoDB.
type InvoiceRepository struct {
	collection *mongo.Collection
}

// New creates an InvoiceRepository and ensures all required indexes exist.
func New(db *mongo.Database) (*InvoiceRepository, error) {
	repo := &InvoiceRepository{
		collection: db.Collection(collectionName),
	}

	if err := repo.ensureIndexes(context.Background()); err != nil {
		return nil, fmt.Errorf("invoice_repo: ensure indexes: %w", err)
	}

	return repo, nil
}

// ensureIndexes creates all required MongoDB indexes idempotently.
func (r *InvoiceRepository) ensureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "invoice_number", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "vendor.name", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "date", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create inserts a new invoice document and returns the assigned ID.
func (r *InvoiceRepository) Create(ctx context.Context, inv *model.Invoice) (*model.Invoice, error) {
	now := time.Now().UTC()
	inv.ID = bson.NewObjectID()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	_, err := r.collection.InsertOne(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: create: %w", err)
	}

	return inv, nil
}

// GetByID retrieves a single invoice by its ObjectID.
func (r *InvoiceRepository) GetByID(ctx context.Context, id bson.ObjectID) (*model.Invoice, error) {
	var inv model.Invoice
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&inv)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("invoice_repo: not found: %s", id.Hex())
		}
		return nil, fmt.Errorf("invoice_repo: get_by_id: %w", err)
	}
	return &inv, nil
}

// List returns a paginated slice of invoices ordered by created_at descending.
func (r *InvoiceRepository) List(ctx context.Context, skip, limit int64) ([]*model.Invoice, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit)

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: list: %w", err)
	}
	defer cursor.Close(ctx)

	var invoices []*model.Invoice
	if err := cursor.All(ctx, &invoices); err != nil {
		return nil, fmt.Errorf("invoice_repo: list decode: %w", err)
	}

	return invoices, nil
}

// Update performs a partial update on an existing invoice document.
func (r *InvoiceRepository) Update(ctx context.Context, id bson.ObjectID, update bson.M) (*model.Invoice, error) {
	update["updated_at"] = time.Now().UTC()

	result, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": update},
	)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: update: %w", err)
	}
	if result.MatchedCount == 0 {
		return nil, fmt.Errorf("invoice_repo: update: not found: %s", id.Hex())
	}

	return r.GetByID(ctx, id)
}

// Delete removes an invoice document by ID.
func (r *InvoiceRepository) Delete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("invoice_repo: delete: %w", err)
	}
	return nil
}
