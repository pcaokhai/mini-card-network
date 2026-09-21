// Package config loads and validates gateway configuration from the environment (fail fast on start).
package config

import (
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
	return cfg, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
