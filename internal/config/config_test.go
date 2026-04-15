package config_test

import (
	"os"
	"testing"

	"github.com/involens/invoice-ocr/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Unset any env overrides that might be set in CI.
	os.Unsetenv("INGESTION_PORT")
	os.Unsetenv("API_PORT")
	os.Unsetenv("MONGO_URI")
	os.Unsetenv("MONGO_DB")
	os.Unsetenv("LLM_PROVIDER")
	os.Unsetenv("CLAUDE_MODEL")
	os.Unsetenv("STORAGE_PATH")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	checks := map[string]string{
		"IngestionPort": cfg.IngestionPort,
		"APIPort":       cfg.APIPort,
		"MongoURI":      cfg.MongoURI,
		"MongoDB":       cfg.MongoDB,
		"LLMProvider":   cfg.LLMProvider,
		"ClaudeModel":   cfg.ClaudeModel,
		"StoragePath":   cfg.StoragePath,
	}
	defaults := map[string]string{
		"IngestionPort": "8080",
		"APIPort":       "8081",
		"MongoURI":      "mongodb://localhost:27017",
		"MongoDB":       "invoices",
		"LLMProvider":   "claude",
		"ClaudeModel":   "claude-sonnet-4-6",
		"StoragePath":   "./storage",
	}
	for field, got := range checks {
		want := defaults[field]
		if got != want {
			t.Errorf("Config.%s = %q, want %q", field, got, want)
		}
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "mock")
	t.Setenv("INGESTION_PORT", "9090")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "mock" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "mock")
	}
	if cfg.IngestionPort != "9090" {
		t.Errorf("IngestionPort = %q, want %q", cfg.IngestionPort, "9090")
	}
}
