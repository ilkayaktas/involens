package gemini_test

import (
	"testing"

	"github.com/involens/invoice-ocr/internal/config"
	"github.com/involens/invoice-ocr/internal/llm/gemini"
)

func TestNew_EmptyAPIKey(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "",
		GeminiModel:  "gemini-2.0-flash",
	}
	_, err := gemini.New(cfg)
	if err == nil {
		t.Fatal("New() with empty API key should return error, got nil")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "AIzaSyFakeKeyForTestingOnly",
		GeminiModel:  "gemini-2.0-flash",
	}
	c, err := gemini.New(cfg)
	if err != nil {
		t.Fatalf("New() with valid config returned error: %v", err)
	}
	if c == nil {
		t.Fatal("New() returned nil client")
	}
}

func TestClient_Name(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "AIzaSyFakeKeyForTestingOnly",
		GeminiModel:  "gemini-2.0-flash",
	}
	c, err := gemini.New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := c.Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}
