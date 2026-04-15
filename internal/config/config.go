package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server
	IngestionPort string // INGESTION_PORT, default "8080"
	APIPort       string // API_PORT, default "8081"

	// MongoDB
	MongoURI string // MONGO_URI, default "mongodb://localhost:27017"
	MongoDB  string // MONGO_DB, default "invoices"

	// LLM Provider (configurable!)
	LLMProvider string // LLM_PROVIDER: "claude" | "openai" | "gemini" | "mock", default "claude"

	// Claude
	AnthropicAPIKey string // ANTHROPIC_API_KEY
	ClaudeModel     string // CLAUDE_MODEL, default "claude-sonnet-4-6"

	// Image Storage
	StoragePath string // STORAGE_PATH, default "./storage"

	// Async worker pool
	WorkerCount int // WORKER_COUNT, default 4
}

// Load reads the .env file (if present) and then reads environment variables into a Config.
func Load() (*Config, error) {
	// Best-effort .env loading — ignore error if file doesn't exist.
	_ = godotenv.Load()

	cfg := &Config{
		IngestionPort:   getEnv("INGESTION_PORT", "8080"),
		APIPort:         getEnv("API_PORT", "8081"),
		MongoURI:        getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:         getEnv("MONGO_DB", "invoices"),
		LLMProvider:     getEnv("LLM_PROVIDER", "claude"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		ClaudeModel:     getEnv("CLAUDE_MODEL", "claude-sonnet-4-6"),
		StoragePath:     getEnv("STORAGE_PATH", "./storage"),
		WorkerCount:     getEnvInt("WORKER_COUNT", 4),
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}
