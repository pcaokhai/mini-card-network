// Package config loads and validates gateway configuration from the environment (fail fast on start).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Config is the validated runtime configuration.
type Config struct {
	ServiceName     string
	HTTPAddr        string
	MetricsAddr     string
	ShutdownTimeout time.Duration
	TracingEnabled  bool
	IssuerAddr      string
	DatabaseURL     string
	SafEncKey       []byte
}

// Load reads configuration through getenv (os.Getenv in production, a map in tests).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ServiceName:    valueOr(getenv("OTEL_SERVICE_NAME"), "gateway"),
		HTTPAddr:       valueOr(getenv("HTTP_ADDR"), ":8080"),
		MetricsAddr:    valueOr(getenv("METRICS_ADDR"), ":9464"),
		TracingEnabled: getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
		IssuerAddr:     valueOr(getenv("ISSUER_ADDR"), "toxiproxy:18000"), // docs/02 §8: gateway connects through Toxiproxy
		DatabaseURL:    getenv("DATABASE_URL"),
	}
	timeout, err := time.ParseDuration(valueOr(getenv("SHUTDOWN_TIMEOUT"), "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return Config{}, errors.New("SHUTDOWN_TIMEOUT must be positive")
	}
	cfg.ShutdownTimeout = timeout

	if raw := getenv("SAF_ENC_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SAF_ENC_KEY: not valid base64: %w", err)
		}
		if len(key) != 32 {
			return Config{}, fmt.Errorf("SAF_ENC_KEY: must decode to 32 bytes (AES-256), got %d", len(key))
		}
		cfg.SafEncKey = key
	}
	// ponytail: an unset SAF_ENC_KEY leaves cfg.SafEncKey nil, and saf.encodePayload/decodePayload
	// treat a nil key as "store the SAF payload unencrypted" - acceptable for local/dev
	// docker-compose where nothing but this repo ever reads the volume; production deployments
	// must set SAF_ENC_KEY (docs/10-engineering-standards.md §2 PCI DSS).

	return cfg, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
