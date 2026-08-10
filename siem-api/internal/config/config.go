package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Addr                   string
	DatabaseURL            string
	LokiURL                string
	LokiJobLabel           string
	VectorGraphQLURL       string
	NtfyURL                string
	NtfyTopic              string
	NtfyToken              string
	OIDCIssuer             string
	OIDCClientID           string
	OIDCGroupsScope        string
	GeoIPDB                string
	SessionSecret          []byte
	FastpathToken          string
	LocalAdminUsername     string
	LocalAdminPasswordHash string
	// AppURL is siem-web's public URL (e.g. "https://siem.example.com").
	// Optional: when set, ntfy notifications get a click-through link, a
	// matching action button, and the app icon; when unset, notifications
	// are still sent, just without those three fields.
	AppURL string
	// OllamaURL/OllamaModel configure the optional LLM-powered insights
	// feature (siem-insights). Both optional: when OllamaURL is unset, the
	// insights scheduler never starts and POST /insights/generate 400s,
	// same degrade-gracefully posture as ntfy being unconfigured.
	OllamaURL   string
	OllamaModel string
	// OllamaTimeoutSec bounds each Chat call's HTTP client timeout - a
	// 20-30B model's first request after Ollama hasn't served anything
	// recently can genuinely take minutes just to load into memory before
	// generation even starts, well beyond a "normal" API call's timeout.
	OllamaTimeoutSec    int
	InsightsIntervalSec int
	InsightsLookbackMin int
}

func Load() (Config, error) {
	cfg := Config{
		Addr:             getenv("ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		LokiURL:          os.Getenv("LOKI_URL"),
		LokiJobLabel:     getenv("LOKI_JOB_LABEL", "siem"),
		VectorGraphQLURL: getenv("VECTOR_GRAPHQL_URL", "http://siem-ingest:8686"),
		NtfyURL:          os.Getenv("NTFY_URL"),
		NtfyTopic:        os.Getenv("NTFY_TOPIC"),
		NtfyToken:        os.Getenv("NTFY_TOKEN"),
		OIDCIssuer:       os.Getenv("OIDC_ISSUER"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCGroupsScope:  getenv("OIDC_GROUPS_SCOPE", "groups"),
		GeoIPDB:          os.Getenv("GEOIP_DB"),
		FastpathToken:    os.Getenv("SIEM_FASTPATH_TOKEN"),
		AppURL:           os.Getenv("APP_URL"),

		OllamaURL:           os.Getenv("OLLAMA_URL"),
		OllamaModel:         os.Getenv("OLLAMA_MODEL"),
		OllamaTimeoutSec:    getenvInt("OLLAMA_TIMEOUT_SEC", 300),
		InsightsIntervalSec: getenvInt("INSIGHTS_INTERVAL_SEC", 1800),
		InsightsLookbackMin: getenvInt("INSIGHTS_LOOKBACK_MIN", 60),

		LocalAdminUsername:     os.Getenv("SIEM_LOCAL_ADMIN_USERNAME"),
		LocalAdminPasswordHash: os.Getenv("SIEM_LOCAL_ADMIN_PASSWORD_HASH"),
	}

	required := map[string]string{
		"DATABASE_URL":        cfg.DatabaseURL,
		"LOKI_URL":            cfg.LokiURL,
		"NTFY_URL":            cfg.NtfyURL,
		"NTFY_TOPIC":          cfg.NtfyTopic,
		"OIDC_ISSUER":         cfg.OIDCIssuer,
		"OIDC_CLIENT_ID":      cfg.OIDCClientID,
		"SIEM_FASTPATH_TOKEN": cfg.FastpathToken,
	}
	for name, val := range required {
		if val == "" {
			return Config{}, fmt.Errorf("config: %s is required", name)
		}
	}

	secretRaw := os.Getenv("SIEM_SESSION_SECRET")
	if secretRaw == "" {
		return Config{}, fmt.Errorf("config: SIEM_SESSION_SECRET is required")
	}
	secret, err := base64.StdEncoding.DecodeString(secretRaw)
	if err != nil {
		return Config{}, fmt.Errorf("config: SIEM_SESSION_SECRET is not valid base64: %w", err)
	}
	cfg.SessionSecret = secret

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
