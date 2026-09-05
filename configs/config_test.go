package configs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMateConfig(t *testing.T) {
	t.Setenv(STUDY_DIR, t.TempDir())
	t.Setenv(OUTPUT_DIR, t.TempDir())
	t.Setenv(STATE_DB, t.TempDir()+"/state.db")
	t.Setenv(DPI, "300")
	t.Setenv(LLM_MODEL, "custom-model")
	t.Setenv(LLM_API_KEY, "sk-plain")

	cfg, err := LoadMateConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DPI != 300 || cfg.LLMModel != "custom-model" || cfg.LLMAPIKey.Value != "sk-plain" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLLMAPIKeyFromFile(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "llm_api_key")
	if err := os.WriteFile(keyFile, []byte("sk-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(LLM_API_KEY, "")
	t.Setenv(LLM_API_KEY_FILE, keyFile)

	key, err := GetLLMAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key.Value != "sk-from-file" {
		t.Fatalf("unexpected key from file: %q", key.Value)
	}
}

func TestLLMAPIKeyRequired(t *testing.T) {
	t.Setenv(LLM_API_KEY, "")
	t.Setenv(LLM_API_KEY_FILE, "")

	if _, err := GetLLMAPIKey(); err == nil {
		t.Fatal("expected missing API key error")
	}
}

func TestRedactedNeverPrintsValue(t *testing.T) {
	secret := RedactedString{Value: "sk-super-secret"}
	rendered := fmt.Sprintf("%v %+v %s", secret, MateConfig{LLMAPIKey: secret}, secret)
	if strings.Contains(rendered, "sk-super-secret") {
		t.Fatalf("secret leaked into formatted output: %s", rendered)
	}
	if !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got: %s", rendered)
	}
}

func TestMateConfigRejectsInvalidDPI(t *testing.T) {
	if _, err := ToRenderDPIFromString("20"); err == nil {
		t.Fatal("expected invalid DPI error")
	}
}
