package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/handler"
	"github.com/involens/invoice-ocr/internal/llm"
	"github.com/involens/invoice-ocr/internal/llm/claude"
	"github.com/involens/invoice-ocr/internal/llm/mock"
	"github.com/involens/invoice-ocr/internal/repository"
	"github.com/involens/invoice-ocr/internal/service"
	"github.com/involens/invoice-ocr/internal/worker"
)

func main() {
	// 1. Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("ingestion: load config: %v", err)
	}

	// 2. Connect to MongoDB with retry.
	db, err := connectMongo(cfg)
	if err != nil {
		log.Fatalf("ingestion: connect mongo: %v", err)
	}

	// 3. Initialize repositories.
	repo, err := repository.New(db)
	if err != nil {
		log.Fatalf("ingestion: init repository: %v", err)
	}

	jobRepo, err := repository.NewJobRepository(db)
	if err != nil {
		log.Fatalf("ingestion: init job repository: %v", err)
	}

	// 4. Initialize LLM extractor via factory pattern.
	extractor, err := newExtractor(cfg)
	if err != nil {
		log.Fatalf("ingestion: init extractor: %v", err)
	}
	log.Printf("ingestion: using LLM provider %q", extractor.Name())

	// 5. Wire service.
	svc := service.New(repo, extractor, cfg.StoragePath)

	// 6. Create and start worker pool.
	workerCount := cfg.WorkerCount
	log.Printf("ingestion: starting worker pool with %d workers", workerCount)
	pool := worker.NewPool(workerCount, svc, jobRepo)
	ctx := context.Background()
	pool.Start(ctx)

	// 7. Wire handler with async support.
	h := handler.NewIngestionHandlerWithAsync(svc, jobRepo, pool)

	// 8. Set up Gin router.
	r := gin.Default()
	h.RegisterRoutes(r)

	// 9. Start HTTP server.
	addr := fmt.Sprintf(":%s", cfg.IngestionPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	log.Printf("ingestion: listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ingestion: server error: %v", err)
	}
}

// connectMongo establishes a MongoDB connection with exponential back-off.
func connectMongo(cfg *config.Config) (*mongo.Database, error) {
	const maxAttempts = 5
	var client *mongo.Client
	var err error

	for i := range maxAttempts {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		client, err = mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
		if err == nil {
			err = client.Ping(ctx, nil)
		}
		cancel()

		if err == nil {
			log.Printf("ingestion: connected to MongoDB at %s", cfg.MongoURI)
			return client.Database(cfg.MongoDB), nil
		}

		// Disconnect the unusable client to avoid leaking connection pools.
		if client != nil {
			_ = client.Disconnect(context.Background())
		}

		wait := time.Duration(1<<uint(i)) * time.Second
		log.Printf("ingestion: MongoDB not ready (attempt %d/%d), retrying in %s: %v", i+1, maxAttempts, wait, err)
		time.Sleep(wait)
	}

	return nil, fmt.Errorf("could not connect to MongoDB after %d attempts: %w", maxAttempts, err)
}

// newExtractor is the factory that selects the LLM provider from config.
func newExtractor(cfg *config.Config) (llm.InvoiceExtractor, error) {
	factories := map[string]llm.ExtractorFactory{
		"claude": func(c *config.Config) (llm.InvoiceExtractor, error) { return claude.New(c) },
		"mock":   func(c *config.Config) (llm.InvoiceExtractor, error) { return mock.New(c) },
	}

	factory, ok := factories[cfg.LLMProvider]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider %q — supported: claude, mock", cfg.LLMProvider)
	}

	return factory(cfg)
}
