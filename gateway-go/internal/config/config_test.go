package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

const envSafEncKey = "SAF_ENC_KEY"

func TestLoad_defaults__MCN_005_AC1(t *testing.T) {
	cfg, err := Load(env(nil))
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, ":9464", cfg.MetricsAddr)
	require.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	require.False(t, cfg.TracingEnabled)
	require.Equal(t, "toxiproxy:18000", cfg.IssuerAddr)
	require.Empty(t, cfg.DatabaseURL)
}

func TestLoad_overrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"HTTP_ADDR": ":18080", "SHUTDOWN_TIMEOUT": "5s", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4317",
		"ISSUER_ADDR": "issuer:9000", "DATABASE_URL": "postgres://acquirer:test@db:5432/acquirer",
	}))
	require.NoError(t, err)
	require.Equal(t, ":18080", cfg.HTTPAddr)
	require.Equal(t, 5*time.Second, cfg.ShutdownTimeout)
	require.True(t, cfg.TracingEnabled)
	require.Equal(t, "issuer:9000", cfg.IssuerAddr)
	require.Equal(t, "postgres://acquirer:test@db:5432/acquirer", cfg.DatabaseURL)
}

func TestLoad_rejectsInvalidValues(t *testing.T) {
	_, err := Load(env(map[string]string{"SHUTDOWN_TIMEOUT": "soon"}))
	require.ErrorContains(t, err, "SHUTDOWN_TIMEOUT")
	_, err = Load(env(map[string]string{"SHUTDOWN_TIMEOUT": "0s"}))
	require.ErrorContains(t, err, "must be positive")
}

func TestLoad_chaosFakeIssuerAddr__MCN_407(t *testing.T) {
	cfg, err := Load(env(map[string]string{"CHAOS_FAKE_ISSUER_ADDR": "127.0.0.1:19999"}))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:19999", cfg.ChaosFakeIssuerAddr)

	cfg, err = Load(env(nil))
	require.NoError(t, err)
	require.Empty(t, cfg.ChaosFakeIssuerAddr)
}

func TestLoad_safEncKeyMustBe32BytesBase64__MCN_401(t *testing.T) {
	cfg, err := Load(env(nil))
	require.NoError(t, err)
	require.Empty(t, cfg.SafEncKey) // unset is allowed (dev/local)

	_, err = Load(env(map[string]string{envSafEncKey: "not-base64!!"}))
	require.ErrorContains(t, err, envSafEncKey)

	_, err = Load(env(map[string]string{envSafEncKey: "dG9vc2hvcnQ="})) // "tooshort", 8 bytes
	require.ErrorContains(t, err, "32 bytes")

	cfg, err = Load(env(map[string]string{envSafEncKey: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="})) // 32 bytes
	require.NoError(t, err)
	require.Len(t, cfg.SafEncKey, 32)
}
