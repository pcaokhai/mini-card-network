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

// testLMKHex is a valid 32-byte (64 hex char) LMK, required by every Load call since MCN-501-AC3.
const testLMKHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e"

// testZMKHex is a valid hex ZMK, required by every Load call since MCN-504-AC1.
const testZMKHex = "3286f59aa74092995a585fd944be6c794b2de041874f2752c5a7895502d472b8" //gitleaks:allow

func withLMK(m map[string]string) map[string]string {
	merged := map[string]string{"LMK_TEST_VALUE_HEX": testLMKHex, "ZMK_HEX": testZMKHex}
	for k, v := range m {
		merged[k] = v
	}
	return merged
}

func TestLoad_defaults__MCN_005_AC1(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, ":9464", cfg.MetricsAddr)
	require.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	require.False(t, cfg.TracingEnabled)
	require.Equal(t, "toxiproxy:18000", cfg.IssuerAddr)
	require.Empty(t, cfg.DatabaseURL)
}

func TestLoad_overrides(t *testing.T) {
	cfg, err := Load(env(withLMK(map[string]string{
		"HTTP_ADDR": ":18080", "SHUTDOWN_TIMEOUT": "5s", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4317",
		"ISSUER_ADDR": "issuer:9000", "DATABASE_URL": "postgres://acquirer:test@db:5432/acquirer",
	})))
	require.NoError(t, err)
	require.Equal(t, ":18080", cfg.HTTPAddr)
	require.Equal(t, 5*time.Second, cfg.ShutdownTimeout)
	require.True(t, cfg.TracingEnabled)
	require.Equal(t, "issuer:9000", cfg.IssuerAddr)
	require.Equal(t, "postgres://acquirer:test@db:5432/acquirer", cfg.DatabaseURL)
}

func TestLoad_rejectsInvalidValues(t *testing.T) {
	_, err := Load(env(withLMK(map[string]string{"SHUTDOWN_TIMEOUT": "soon"})))
	require.ErrorContains(t, err, "SHUTDOWN_TIMEOUT")
	_, err = Load(env(withLMK(map[string]string{"SHUTDOWN_TIMEOUT": "0s"})))
	require.ErrorContains(t, err, "must be positive")
}

func TestLoad_chaosFakeIssuerAddr__MCN_407(t *testing.T) {
	cfg, err := Load(env(withLMK(map[string]string{"CHAOS_FAKE_ISSUER_ADDR": "127.0.0.1:19999"})))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:19999", cfg.ChaosFakeIssuerAddr)

	cfg, err = Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Empty(t, cfg.ChaosFakeIssuerAddr)
}

func TestLoad_safEncKeyMustBe32BytesBase64__MCN_401(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Empty(t, cfg.SafEncKey) // unset is allowed (dev/local)

	_, err = Load(env(withLMK(map[string]string{envSafEncKey: "not-base64!!"})))
	require.ErrorContains(t, err, envSafEncKey)

	_, err = Load(env(withLMK(map[string]string{envSafEncKey: "dG9vc2hvcnQ="}))) // "tooshort", 8 bytes
	require.ErrorContains(t, err, "32 bytes")

	cfg, err = Load(env(withLMK(map[string]string{envSafEncKey: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="}))) // 32 bytes, decodes to "01234567890123456789012345678901" - not a real key //gitleaks:allow
	require.NoError(t, err)
	require.Len(t, cfg.SafEncKey, 32)
}

func TestLoad_requiresLMKTestValueHex__MCN_501_AC3(t *testing.T) {
	_, err := Load(env(nil))
	require.ErrorContains(t, err, "LMK_TEST_VALUE_HEX")

	cfg, err := Load(env(map[string]string{"LMK_TEST_VALUE_HEX": testLMKHex, "ZMK_HEX": testZMKHex}))
	require.NoError(t, err)
	require.NotEmpty(t, cfg.LMKTestValueHex)
}

func TestLoad_requiresZMKHex__MCN_504_AC1(t *testing.T) {
	_, err := Load(env(map[string]string{"LMK_TEST_VALUE_HEX": testLMKHex}))
	require.ErrorContains(t, err, "ZMK_HEX")

	_, err = Load(env(map[string]string{"LMK_TEST_VALUE_HEX": testLMKHex, "ZMK_HEX": "not-hex!!"}))
	require.ErrorContains(t, err, "ZMK_HEX")

	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.NotEmpty(t, cfg.ZMK)
}
