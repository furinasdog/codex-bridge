package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddress      = "127.0.0.1:8787"
	defaultUpstreamURL  = "https://api.openai.com/v1"
	defaultModel        = "gpt-6.1-sol"
	defaultRequestLimit = 32 << 20
)

type Config struct {
	Address          string
	UpstreamURL      string
	UpstreamModel    string
	CredentialPath   string
	AccessToken      string
	APIKey           string
	AllowedOrigins   []string
	RequestBodyLimit int64
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
	OAuthTimeout     time.Duration
	LogLevel         string
	ForwardUserAgent string
}

func Load() (Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve user config directory: %w", err)
	}

	cfg := Config{
		Address:          env("CODEX_BRIDGE_ADDRESS", defaultAddress),
		UpstreamURL:      strings.TrimRight(env("CODEX_BRIDGE_UPSTREAM_URL", defaultUpstreamURL), "/"),
		UpstreamModel:    env("CODEX_BRIDGE_MODEL", defaultModel),
		CredentialPath:   env("CODEX_BRIDGE_CREDENTIALS", filepath.Join(configDir, "codex-bridge", "credentials.json")),
		AccessToken:      os.Getenv("CODEX_ACCESS_TOKEN"),
		APIKey:           os.Getenv("CODEX_BRIDGE_API_KEY"),
		AllowedOrigins:   splitCSV(os.Getenv("CODEX_BRIDGE_ALLOWED_ORIGINS")),
		RequestBodyLimit: defaultRequestLimit,
		RequestTimeout:   10 * time.Minute,
		ShutdownTimeout:  10 * time.Second,
		OAuthTimeout:     5 * time.Minute,
		LogLevel:         env("CODEX_BRIDGE_LOG_LEVEL", "info"),
		ForwardUserAgent: "codex-bridge",
	}

	if raw := os.Getenv("CODEX_BRIDGE_MAX_BODY_BYTES"); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value <= 0 {
			return Config{}, fmt.Errorf("CODEX_BRIDGE_MAX_BODY_BYTES must be a positive integer")
		}
		cfg.RequestBodyLimit = value
	}
	if raw := os.Getenv("CODEX_BRIDGE_REQUEST_TIMEOUT"); raw != "" {
		value, parseErr := time.ParseDuration(raw)
		if parseErr != nil || value <= 0 {
			return Config{}, fmt.Errorf("CODEX_BRIDGE_REQUEST_TIMEOUT must be a positive duration")
		}
		cfg.RequestTimeout = value
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}
