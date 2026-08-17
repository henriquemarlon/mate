package configs

import "testing"

func TestLoadMateConfig(t *testing.T) {
	t.Setenv(STUDY_DIR, t.TempDir())
	t.Setenv(OUTPUT_DIR, t.TempDir())
	t.Setenv(STATE_DB, t.TempDir()+"/state.db")
	t.Setenv(DPI, "300")
	t.Setenv(CODEX_BIN, "custom-codex")

	cfg, err := LoadMateConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DPI != 300 || cfg.CodexBin != "custom-codex" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestMateConfigRejectsInvalidDPI(t *testing.T) {
	if _, err := ToRenderDPIFromString("20"); err == nil {
		t.Fatal("expected invalid DPI error")
	}
}
