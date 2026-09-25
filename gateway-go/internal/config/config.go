// Package config loads and validates gateway configuration from the environment (fail fast on start).
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"

	"strconv"
	"strings"
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
	IssuerAdminURL string
	// ChaosRunMaxDuration caps a chaos run's wall-clock time (CHAOS_RUN_MAX_DURATION, default 10m).
	ChaosRunMaxDuration time.Duration
	LMKTestValueHex     string
	ZMK                 []byte
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
	// KeyLifetimeDays is the rotation policy per key type (KEY_LIFETIME_DAYS, e.g. "ZPK=30,ZAK=30"),
	// behind GET /v1/keys/acquirer's lifetimeDays and daysRemaining (SEC-G9).
	KeyLifetimeDays map[string]int
}

// defaultKeyLifetimes: working keys rotate monthly, the ZMK yearly (the Security page's canvas).
const defaultKeyLifetimes = "ZMK=365,ZPK=30,ZAK=30"

// maxKeyLifetimeDays rejects a typo like 3000 → 30000; ten years is beyond any key policy here.
const maxKeyLifetimeDays = 3650

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
	var err error
	if cfg.ShutdownTimeout, err = positiveDuration(getenv, "SHUTDOWN_TIMEOUT", "30s"); err != nil {
		return Config{}, err
	}
	if cfg.ChaosRunMaxDuration, err = positiveDuration(getenv, "CHAOS_RUN_MAX_DURATION", "10m"); err != nil {
		return Config{}, err
	}

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
	lifetimes, err := keyLifetimes(valueOr(getenv("KEY_LIFETIME_DAYS"), defaultKeyLifetimes))
	if err != nil {
		return fmt.Errorf("KEY_LIFETIME_DAYS: %w", err)
	}
	cfg.KeyLifetimeDays = lifetimes
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

// positiveDuration reads name as a Go duration (fallback when unset) that must be above zero.
func positiveDuration(getenv func(string) string, name, fallback string) (time.Duration, error) {
	d, err := time.ParseDuration(valueOr(getenv(name), fallback))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return d, nil
}

// httpURL accepts only an absolute http(s) URL with a host.
func httpURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("want an http(s) URL with a host, got %q", raw)
	}
	return raw, nil
}

// keyLifetimes parses "TYPE=days,..." for the contract's KeyInfo key types.
func keyLifetimes(raw string) (map[string]int, error) {
	known := map[string]bool{"ZMK": true, "ZPK": true, "ZAK": true, "TPK": true, "TAK": true, "CVK": true, "PVK": true}
	out := map[string]int{}
	for _, pair := range strings.Split(raw, ",") {
		keyType, daysText, ok := strings.Cut(strings.TrimSpace(pair), "=")
		days, err := strconv.Atoi(daysText)
		switch {
		case !ok || err != nil:
			return nil, fmt.Errorf("want TYPE=days pairs, got %q", pair)
		case !known[keyType]:
			return nil, fmt.Errorf("unknown key type %q", keyType)
		case days < 1 || days > maxKeyLifetimeDays:
			return nil, fmt.Errorf("%s: days must be 1-%d, got %d", keyType, maxKeyLifetimeDays, days)
		}
		if _, dup := out[keyType]; dup {
			return nil, fmt.Errorf("%s listed twice", keyType)
		}
		out[keyType] = days
	}
	return out, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
