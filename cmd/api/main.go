package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
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
)

func main() {
	// 1. Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("api: load config: %v", err)
	}

	// 2. Connect to MongoDB with retry.
	db, err := connectMongo(cfg)
	if err != nil {
		log.Fatalf("api: connect mongo: %v", err)
	}

	// 3. Initialize repository.
	repo, err := repository.New(db)
	if err != nil {
		log.Fatalf("api: init repository: %v", err)
	}

	// 4. Initialize LLM extractor via factory pattern (matches ingestion service behaviour).
	extractor, err := newExtractor(cfg)
	if err != nil {
		log.Fatalf("api: init extractor: %v", err)
	}
	log.Printf("api: using LLM provider %q", extractor.Name())

	// 5. Wire service and handler.
	svc := service.New(repo, extractor, cfg.StoragePath)
	h := handler.NewAPIHandler(svc, db)

	// 6. Set up Gin router with CORS middleware.
	r := gin.Default()

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{cfg.CORSOrigin}
	corsConfig.AllowMethods = []string{"GET", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	r.Use(cors.New(corsConfig))

	h.RegisterRoutes(r)

	// 7. Start HTTP server with graceful shutdown.
	addr := fmt.Sprintf(":%s", cfg.APIPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("api: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: server error: %v", err)
		}
	}()

	// Wait for OS signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("api: shutting down...")

	// Shutdown with 30s timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("api: forced shutdown: %v", err)
	}

	log.Println("api: stopped")
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
			log.Printf("api: connected to MongoDB at %s", cfg.MongoURI)
			return client.Database(cfg.MongoDB), nil
		}

		// Disconnect the unusable client to avoid leaking connection pools.
		if client != nil {
			_ = client.Disconnect(context.Background())
		}

		wait := time.Duration(1<<uint(i)) * time.Second
		log.Printf("api: MongoDB not ready (attempt %d/%d), retrying in %s: %v", i+1, maxAttempts, wait, err)
		time.Sleep(wait)
	}

	return nil, fmt.Errorf("could not connect to MongoDB after %d attempts: %w", maxAttempts, err)
}

// newExtractor selects the LLM provider from config using the factory pattern.
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
