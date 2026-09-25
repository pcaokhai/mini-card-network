// Package config loads and validates gateway configuration from the environment (fail fast on start).
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
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
	// IssuerAdminURL is the Issuer Admin API base URL; chaos runs read card balances from it (CHA-G1).
	IssuerAdminURL  string
	LMKTestValueHex string
	ZMK             []byte
	// InitialZAK / InitialZPK are the working keys the issuer is configured with. When
	// key_store has no ACTIVE key of that type, the gateway registers these at startup so a
	// fresh stack can MAC and verify from the first purchase; nil means "provision nothing".
	InitialZAK []byte
	InitialZPK []byte
	// CORSAllowedOrigin is empty by default (CORS middleware off - safe for a topology where a
	// server-side BFF calls the gateway, never a browser directly). Set it only for local dev
	// where web-next talks to the gateway straight from the browser (docs/09-risk-register.md
	// R-10 - the real BFF proxy this workaround stands in for doesn't exist yet).
	CORSAllowedOrigin string
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
		CORSAllowedOrigin:   getenv("CORS_ALLOWED_ORIGIN"),                  // empty means off (R-10)
	}
	timeout, err := time.ParseDuration(valueOr(getenv("SHUTDOWN_TIMEOUT"), "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return Config{}, errors.New("SHUTDOWN_TIMEOUT must be positive")
	}
	cfg.ShutdownTimeout = timeout

	if cfg.IssuerAdminURL, err = httpURL(valueOr(getenv("ISSUER_ADMIN_URL"), "http://issuer:8081")); err != nil {
		return Config{}, fmt.Errorf("ISSUER_ADMIN_URL: %w", err)
	}

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

	if err := loadOptionalKeys(getenv, &cfg); err != nil {
		return Config{}, err
	}
	// ponytail: an unset SAF_ENC_KEY leaves cfg.SafEncKey nil, and saf.encodePayload/decodePayload
	// treat a nil key as "store the SAF payload unencrypted" - acceptable for local/dev
	// docker-compose where nothing but this repo ever reads the volume; production deployments
	// must set SAF_ENC_KEY (docs/10-engineering-standards.md §2 PCI DSS).

	return cfg, nil
}

// loadOptionalKeys reads the keys a lab stack may leave unset: SAF_ENC_KEY (base64, 32 bytes)
// and the initial working keys ZAK_HEX / ZPK_HEX (hex AES).
func loadOptionalKeys(getenv func(string) string, cfg *Config) error {
	if raw := getenv("SAF_ENC_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("SAF_ENC_KEY: not valid base64: %w", err)
		}
		if len(key) != 32 {
			return fmt.Errorf("SAF_ENC_KEY: must decode to 32 bytes (AES-256), got %d", len(key))
		}
		cfg.SafEncKey = key
	}
	var err error
	if cfg.InitialZAK, err = optionalAESKeyHex(getenv, "ZAK_HEX"); err != nil {
		return err
	}
	cfg.InitialZPK, err = optionalAESKeyHex(getenv, "ZPK_HEX")
	return err
}

// optionalAESKeyHex decodes name as a hex AES key (16, 24 or 32 bytes); unset returns nil.
func optionalAESKeyHex(getenv func(string) string, name string) ([]byte, error) {
	raw := getenv(name)
	if raw == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: not valid hex: %w", name, err)
	}
	switch len(key) {
	case 16, 24, 32:
		return key, nil
	default:
		return nil, fmt.Errorf("%s: must decode to 16, 24 or 32 bytes (AES), got %d", name, len(key))
	}
}

// httpURL accepts only an absolute http(s) URL with a host.
func httpURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("want an http(s) URL with a host, got %q", raw)
	}
	return raw, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
