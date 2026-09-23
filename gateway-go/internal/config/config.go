// Package config loads and validates gateway configuration from the environment (fail fast on start).
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Config is the validated runtime configuration.
type Config struct {
	ServiceName         string
	HTTPAddr            string
	MetricsAddr         string
	ShutdownTimeout     time.Duration
	TracingEnabled      bool
	IssuerAddr          string
	DatabaseURL         string
	SafEncKey           []byte
	ToxiproxyAdminAddr  string
	IssuerProxyName     string
	ChaosFakeIssuerAddr string
	LMKTestValueHex     string
	ZMK                 []byte
}

// Load reads configuration through getenv (os.Getenv in production, a map in tests).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ServiceName:         valueOr(getenv("OTEL_SERVICE_NAME"), "gateway"),
		HTTPAddr:            valueOr(getenv("HTTP_ADDR"), ":8080"),
		MetricsAddr:         valueOr(getenv("METRICS_ADDR"), ":9464"),
		TracingEnabled:      getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
		IssuerAddr:          valueOr(getenv("ISSUER_ADDR"), "toxiproxy:18000"), // docs/02 §8: gateway connects through Toxiproxy
		DatabaseURL:         getenv("DATABASE_URL"),
		ToxiproxyAdminAddr:  valueOr(getenv("TOXIPROXY_ADMIN_ADDR"), "http://toxiproxy:8474"),
		IssuerProxyName:     valueOr(getenv("ISSUER_PROXY_NAME"), "issuer"), // infra/toxiproxy/toxiproxy.json
		ChaosFakeIssuerAddr: getenv("CHAOS_FAKE_ISSUER_ADDR"),               // empty means off (MCN-407)
	}
	timeout, err := time.ParseDuration(valueOr(getenv("SHUTDOWN_TIMEOUT"), "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return Config{}, errors.New("SHUTDOWN_TIMEOUT must be positive")
	}
	cfg.ShutdownTimeout = timeout

	cfg.LMKTestValueHex = getenv("LMK_TEST_VALUE_HEX")
	if cfg.LMKTestValueHex == "" {
		return Config{}, errors.New("LMK_TEST_VALUE_HEX is required")
	}

	zmkHex := getenv("ZMK_HEX")
	if zmkHex == "" {
		return Config{}, errors.New("ZMK_HEX is required")
	}
	zmk, err := hex.DecodeString(zmkHex)
	if err != nil {
		return Config{}, fmt.Errorf("ZMK_HEX: not valid hex: %w", err)
	}
	cfg.ZMK = zmk

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
