package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/involens/invoice-ocr/internal/model"
)

// ErrNotFound is returned when a requested invoice document does not exist.
var ErrNotFound = errors.New("invoice not found")

// SearchParams defines the filter/sort/pagination options for Search.
type SearchParams struct {
	Vendor    string  // substring match on vendor.name
	From      string  // YYYY-MM-DD date range start (inclusive)
	To        string  // YYYY-MM-DD date range end (inclusive)
	MinAmount float64
	MaxAmount float64
	Status    string
	Currency  string
	SortBy    string // "date" | "total" | "created_at" (default: "created_at")
	SortOrder string // "asc" | "desc" (default: "desc")
	Skip      int64
	Limit     int64
}

// Stats holds aggregate statistics for the invoice collection.
type Stats struct {
	TotalInvoices int64         `json:"total_invoices"`
	TotalAmount   float64       `json:"total_amount"`
	Currency      string        `json:"currency"`
	ByVendor      []VendorStat  `json:"by_vendor"`
	ByMonth       []MonthlyStat `json:"by_month"`
}

// VendorStat holds per-vendor aggregate data.
type VendorStat struct {
	Name  string  `json:"name"`
	Count int64   `json:"count"`
	Total float64 `json:"total"`
}

// MonthlyStat holds per-month aggregate data.
type MonthlyStat struct {
	Month string  `json:"month"` // "YYYY-MM"
	Count int64   `json:"count"`
	Total float64 `json:"total"`
}

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
			return nil, fmt.Errorf("invoice_repo: get_by_id %s: %w", id.Hex(), ErrNotFound)
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
		return nil, fmt.Errorf("invoice_repo: update %s: %w", id.Hex(), ErrNotFound)
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

// Search queries invoices with optional filters, sorting, and pagination.
// It returns the matching page of invoices and the total count of matching documents.
func (r *InvoiceRepository) Search(ctx context.Context, params SearchParams) ([]*model.Invoice, int64, error) {
	filter := bson.D{}

	if params.Vendor != "" {
		filter = append(filter, bson.E{
			Key:   "vendor.name",
			Value: bson.M{"$regex": params.Vendor, "$options": "i"},
		})
	}

	dateFilter := bson.M{}
	if params.From != "" {
		dateFilter["$gte"] = params.From
	}
	if params.To != "" {
		dateFilter["$lte"] = params.To
	}
	if len(dateFilter) > 0 {
		filter = append(filter, bson.E{Key: "date", Value: dateFilter})
	}

	amountFilter := bson.M{}
	if params.MinAmount > 0 {
		amountFilter["$gte"] = params.MinAmount
	}
	if params.MaxAmount > 0 {
		amountFilter["$lte"] = params.MaxAmount
	}
	if len(amountFilter) > 0 {
		filter = append(filter, bson.E{Key: "total", Value: amountFilter})
	}

	if params.Status != "" {
		filter = append(filter, bson.E{Key: "status", Value: params.Status})
	}

	if params.Currency != "" {
		filter = append(filter, bson.E{Key: "currency", Value: params.Currency})
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("invoice_repo: search count: %w", err)
	}

	sortField := "created_at"
	switch params.SortBy {
	case "date":
		sortField = "date"
	case "total":
		sortField = "total"
	}

	sortDir := -1
	if params.SortOrder == "asc" {
		sortDir = 1
	}

	opts := options.Find().
		SetSort(bson.D{{Key: sortField, Value: sortDir}}).
		SetSkip(params.Skip).
		SetLimit(params.Limit)

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("invoice_repo: search find: %w", err)
	}
	defer cursor.Close(ctx)

	var invoices []*model.Invoice
	if err := cursor.All(ctx, &invoices); err != nil {
		return nil, 0, fmt.Errorf("invoice_repo: search decode: %w", err)
	}

	return invoices, total, nil
}

// GetStats returns aggregate statistics for the entire invoice collection.
func (r *InvoiceRepository) GetStats(ctx context.Context) (*Stats, error) {
	// --- total invoices + total amount ---
	totalPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":          nil,
			"totalInvoices": bson.M{"$sum": 1},
			"totalAmount":   bson.M{"$sum": "$total"},
		}}},
	}
	totalCursor, err := r.collection.Aggregate(ctx, totalPipeline)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: stats total: %w", err)
	}
	defer totalCursor.Close(ctx)

	var totalResult struct {
		TotalInvoices int64   `bson:"totalInvoices"`
		TotalAmount   float64 `bson:"totalAmount"`
	}
	if totalCursor.Next(ctx) {
		if err := totalCursor.Decode(&totalResult); err != nil {
			return nil, fmt.Errorf("invoice_repo: stats total decode: %w", err)
		}
	}

	// --- most common currency ---
	currencyPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$currency",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: 1}},
	}
	currCursor, err := r.collection.Aggregate(ctx, currencyPipeline)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: stats currency: %w", err)
	}
	defer currCursor.Close(ctx)

	currency := ""
	var currResult struct {
		ID string `bson:"_id"`
	}
	if currCursor.Next(ctx) {
		if err := currCursor.Decode(&currResult); err != nil {
			return nil, fmt.Errorf("invoice_repo: stats currency decode: %w", err)
		}
		currency = currResult.ID
	}

	// --- by vendor (top 10) ---
	vendorPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$vendor.name",
			"count": bson.M{"$sum": 1},
			"total": bson.M{"$sum": "$total"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
		{{Key: "$limit", Value: 10}},
	}
	vendorCursor, err := r.collection.Aggregate(ctx, vendorPipeline)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: stats vendor: %w", err)
	}
	defer vendorCursor.Close(ctx)

	var byVendor []VendorStat
	for vendorCursor.Next(ctx) {
		var row struct {
			ID    string  `bson:"_id"`
			Count int64   `bson:"count"`
			Total float64 `bson:"total"`
		}
		if err := vendorCursor.Decode(&row); err != nil {
			return nil, fmt.Errorf("invoice_repo: stats vendor decode: %w", err)
		}
		byVendor = append(byVendor, VendorStat{Name: row.ID, Count: row.Count, Total: row.Total})
	}
	if err := vendorCursor.Err(); err != nil {
		return nil, fmt.Errorf("invoice_repo: stats vendor iterate: %w", err)
	}
	if byVendor == nil {
		byVendor = []VendorStat{}
	}

	// --- by month (last 12 months) ---
	twelveMonthsAgo := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01")
	monthPipeline := mongo.Pipeline{
		{{Key: "$addFields", Value: bson.M{
			"monthStr": bson.M{"$substr": bson.A{"$date", 0, 7}},
		}}},
		{{Key: "$match", Value: bson.M{
			"monthStr": bson.M{"$gte": twelveMonthsAgo},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$monthStr",
			"count": bson.M{"$sum": 1},
			"total": bson.M{"$sum": "$total"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	monthCursor, err := r.collection.Aggregate(ctx, monthPipeline)
	if err != nil {
		return nil, fmt.Errorf("invoice_repo: stats month: %w", err)
	}
	defer monthCursor.Close(ctx)

	var byMonth []MonthlyStat
	for monthCursor.Next(ctx) {
		var row struct {
			ID    string  `bson:"_id"`
			Count int64   `bson:"count"`
			Total float64 `bson:"total"`
		}
		if err := monthCursor.Decode(&row); err != nil {
			return nil, fmt.Errorf("invoice_repo: stats month decode: %w", err)
		}
		byMonth = append(byMonth, MonthlyStat{Month: row.ID, Count: row.Count, Total: row.Total})
	}
	if err := monthCursor.Err(); err != nil {
		return nil, fmt.Errorf("invoice_repo: stats month iterate: %w", err)
	}
	if byMonth == nil {
		byMonth = []MonthlyStat{}
	}

	return &Stats{
		TotalInvoices: totalResult.TotalInvoices,
		TotalAmount:   totalResult.TotalAmount,
		Currency:      currency,
		ByVendor:      byVendor,
		ByMonth:       byMonth,
	}, nil
}
