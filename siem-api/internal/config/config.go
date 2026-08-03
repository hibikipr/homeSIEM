package config

import (
	"encoding/base64"
	"fmt"
	"os"
)

type Config struct {
	Addr                   string
	DatabaseURL            string
	LokiURL                string
	LokiJobLabel           string
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
}

func Load() (Config, error) {
	cfg := Config{
		Addr:            getenv("ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		LokiURL:         os.Getenv("LOKI_URL"),
		LokiJobLabel:    getenv("LOKI_JOB_LABEL", "siem"),
		NtfyURL:         os.Getenv("NTFY_URL"),
		NtfyTopic:       os.Getenv("NTFY_TOPIC"),
		NtfyToken:       os.Getenv("NTFY_TOKEN"),
		OIDCIssuer:      os.Getenv("OIDC_ISSUER"),
		OIDCClientID:    os.Getenv("OIDC_CLIENT_ID"),
		OIDCGroupsScope: getenv("OIDC_GROUPS_SCOPE", "groups"),
		GeoIPDB:         os.Getenv("GEOIP_DB"),
		FastpathToken:   os.Getenv("SIEM_FASTPATH_TOKEN"),

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
