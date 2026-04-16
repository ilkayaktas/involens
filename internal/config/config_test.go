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

func TestConfig_GeminiDefaults(t *testing.T) {
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GEMINI_MODEL")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GeminiAPIKey != "" {
		t.Errorf("GeminiAPIKey = %q, want empty string", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "gemini-2.0-flash" {
		t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, "gemini-2.0-flash")
	}
}

func TestConfig_GeminiEnvOverride(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key-123")
	t.Setenv("GEMINI_MODEL", "gemini-2.5-pro")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GeminiAPIKey != "test-key-123" {
		t.Errorf("GeminiAPIKey = %q, want %q", cfg.GeminiAPIKey, "test-key-123")
	}
	if cfg.GeminiModel != "gemini-2.5-pro" {
		t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, "gemini-2.5-pro")
	}
}
