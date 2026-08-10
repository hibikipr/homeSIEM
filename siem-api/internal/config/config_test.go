package config

import (
	"encoding/base64"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	env := map[string]string{
		"DATABASE_URL":        "sqlite:///data/siem.db",
		"LOKI_URL":            "http://loki:3100",
		"NTFY_URL":            "http://ntfy",
		"NTFY_TOPIC":          "homesiem",
		"OIDC_ISSUER":         "https://pocketid.townsville.cc",
		"OIDC_CLIENT_ID":      "homeSIEM",
		"SIEM_SESSION_SECRET": secret,
		"SIEM_FASTPATH_TOKEN": "fastpath-token",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.LokiJobLabel != "siem" {
		t.Errorf("LokiJobLabel = %q, want siem", cfg.LokiJobLabel)
	}
	if cfg.OIDCGroupsScope != "groups" {
		t.Errorf("OIDCGroupsScope = %q, want groups", cfg.OIDCGroupsScope)
	}
	if cfg.VectorGraphQLURL != "http://siem-ingest:8686" {
		t.Errorf("VectorGraphQLURL = %q, want http://siem-ingest:8686", cfg.VectorGraphQLURL)
	}
	if len(cfg.SessionSecret) != 32 {
		t.Errorf("SessionSecret len = %d, want 32", len(cfg.SessionSecret))
	}
	if cfg.AppURL != "" {
		t.Errorf("AppURL = %q, want empty when APP_URL is unset", cfg.AppURL)
	}
	if cfg.OllamaURL != "" {
		t.Errorf("OllamaURL = %q, want empty when OLLAMA_URL is unset", cfg.OllamaURL)
	}
	if cfg.OllamaModel != "" {
		t.Errorf("OllamaModel = %q, want empty when OLLAMA_MODEL is unset", cfg.OllamaModel)
	}
	if cfg.OllamaTimeoutSec != 300 {
		t.Errorf("OllamaTimeoutSec = %d, want 300 (the default)", cfg.OllamaTimeoutSec)
	}
	if cfg.InsightsIntervalSec != 1800 {
		t.Errorf("InsightsIntervalSec = %d, want 1800 (the default)", cfg.InsightsIntervalSec)
	}
	if cfg.InsightsLookbackMin != 60 {
		t.Errorf("InsightsLookbackMin = %d, want 60 (the default)", cfg.InsightsLookbackMin)
	}
}

func TestLoad_InsightsEnvOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OLLAMA_URL", "http://192.168.3.50:11434")
	t.Setenv("OLLAMA_MODEL", "qwen3:27b")
	t.Setenv("OLLAMA_TIMEOUT_SEC", "600")
	t.Setenv("INSIGHTS_INTERVAL_SEC", "900")
	t.Setenv("INSIGHTS_LOOKBACK_MIN", "30")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OllamaURL != "http://192.168.3.50:11434" {
		t.Errorf("OllamaURL = %q, want http://192.168.3.50:11434", cfg.OllamaURL)
	}
	if cfg.OllamaModel != "qwen3:27b" {
		t.Errorf("OllamaModel = %q, want qwen3:27b", cfg.OllamaModel)
	}
	if cfg.OllamaTimeoutSec != 600 {
		t.Errorf("OllamaTimeoutSec = %d, want 600", cfg.OllamaTimeoutSec)
	}
	if cfg.InsightsIntervalSec != 900 {
		t.Errorf("InsightsIntervalSec = %d, want 900", cfg.InsightsIntervalSec)
	}
	if cfg.InsightsLookbackMin != 30 {
		t.Errorf("InsightsLookbackMin = %d, want 30", cfg.InsightsLookbackMin)
	}
}

func TestLoad_InvalidInsightsIntervalFallsBackToDefault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INSIGHTS_INTERVAL_SEC", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.InsightsIntervalSec != 1800 {
		t.Errorf("InsightsIntervalSec = %d, want 1800 (fallback for an unparseable value)", cfg.InsightsIntervalSec)
	}
}

func TestLoad_AppURLReadFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("APP_URL", "https://siem.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppURL != "https://siem.example.com" {
		t.Errorf("AppURL = %q, want https://siem.example.com", cfg.AppURL)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for missing DATABASE_URL")
	}
}

func TestLoad_InvalidSessionSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SIEM_SESSION_SECRET", "not-valid-base64!!")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for invalid base64 secret")
	}
}
