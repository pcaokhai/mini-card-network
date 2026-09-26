package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

const (
	envSafEncKey   = "SAF_ENC_KEY"
	envCutoverTime = "CUTOVER_TIME"
)

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
	require.Empty(t, cfg.CORSAllowedOrigin) // unset means CORS middleware stays off
}

func TestLoad_corsAllowedOrigin__R10(t *testing.T) {
	cfg, err := Load(env(withLMK(map[string]string{"CORS_ALLOWED_ORIGIN": "http://localhost:3000"})))
	require.NoError(t, err)
	require.Equal(t, "http://localhost:3000", cfg.CORSAllowedOrigin)
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

func TestLoad_initialWorkingKeysAreOptionalHexAESKeys__MCN_002(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Nil(t, cfg.InitialZAK, "unset means no provisioning")
	require.Nil(t, cfg.InitialZPK)

	cfg, err = Load(env(withLMK(map[string]string{
		"ZAK_HEX": "3132333435363738393a3b3c3d3e3f40", //gitleaks:allow
		"ZPK_HEX": "2132333435363738393a3b3c3d3e3f41", //gitleaks:allow
	})))
	require.NoError(t, err)
	require.Len(t, cfg.InitialZAK, 16)
	require.Len(t, cfg.InitialZPK, 16)

	_, err = Load(env(withLMK(map[string]string{"ZAK_HEX": "zz"})))
	require.ErrorContains(t, err, "ZAK_HEX")

	_, err = Load(env(withLMK(map[string]string{"ZPK_HEX": "0102030405"})))
	require.ErrorContains(t, err, "ZPK_HEX", "5 bytes is not an AES key length")
}

func TestLoad_issuerAdminURL__CHA_G1(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Equal(t, "http://issuer:8081", cfg.IssuerAdminURL)

	cfg, err = Load(env(withLMK(map[string]string{"ISSUER_ADMIN_URL": "http://localhost:18081"})))
	require.NoError(t, err)
	require.Equal(t, "http://localhost:18081", cfg.IssuerAdminURL)

	for _, bad := range []string{"issuer:8081", "ftp://issuer", "http://", "://x"} {
		_, err = Load(env(withLMK(map[string]string{"ISSUER_ADMIN_URL": bad})))
		require.ErrorContains(t, err, "ISSUER_ADMIN_URL", bad)
	}
}

func TestLoad_chaosRunMaxDuration__CHA_N2(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Equal(t, 10*time.Minute, cfg.ChaosRunMaxDuration)

	cfg, err = Load(env(withLMK(map[string]string{"CHAOS_RUN_MAX_DURATION": "90s"})))
	require.NoError(t, err)
	require.Equal(t, 90*time.Second, cfg.ChaosRunMaxDuration)

	for _, bad := range []string{"soon", "0s", "-1m"} {
		_, err = Load(env(withLMK(map[string]string{"CHAOS_RUN_MAX_DURATION": bad})))
		require.ErrorContains(t, err, "CHAOS_RUN_MAX_DURATION", bad)
	}
}

const typeZPK = "ZPK"

func TestLoad_keyLifetimePolicy__SEC_G9(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Equal(t, map[string]int{"ZMK": 365, "ZPK": 30, "ZAK": 30}, cfg.KeyLifetimeDays)

	cfg, err = Load(env(withLMK(map[string]string{"KEY_LIFETIME_DAYS": "ZPK=7, ZAK=14"})))
	require.NoError(t, err)
	require.Equal(t, map[string]int{"ZPK": 7, "ZAK": 14}, cfg.KeyLifetimeDays)

	for _, bad := range []string{typeZPK, "ZPK=0", "ZPK=abc", "FOO=30", "ZPK=30,ZPK=40", "ZPK=4000"} {
		_, err = Load(env(withLMK(map[string]string{"KEY_LIFETIME_DAYS": bad})))
		require.ErrorContains(t, err, "KEY_LIFETIME_DAYS", bad)
	}
}

func TestLoad_echoTimeoutStaysUnderTheHTTPWriteTimeout__NET_N2(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, cfg.EchoTimeout)

	cfg, err = Load(env(withLMK(map[string]string{"ECHO_TIMEOUT": "15s"})))
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, cfg.EchoTimeout)

	for _, bad := range []string{"0s", "16s", "1m", "later"} {
		_, err = Load(env(withLMK(map[string]string{"ECHO_TIMEOUT": bad})))
		require.ErrorContains(t, err, "ECHO_TIMEOUT", bad)
	}
}

func TestLoad_rotationSendAttempts__SEC_S2(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))
	require.NoError(t, err)
	require.Equal(t, 3, cfg.RotationSendAttempts)

	for _, bad := range []string{"0", "11", "x"} {
		_, err = Load(env(withLMK(map[string]string{"ROTATION_SEND_ATTEMPTS": bad})))
		require.ErrorContains(t, err, "ROTATION_SEND_ATTEMPTS", bad)
	}
}

func TestLoad_cutoverDefaultsToOneSecondBeforeLocalMidnight__OVW_G7(t *testing.T) {
	cfg, err := Load(env(withLMK(nil)))

	require.NoError(t, err)
	require.Equal(t, 23*time.Hour+59*time.Minute+59*time.Second, cfg.CutoverTime)
	require.Equal(t, "Asia/Ho_Chi_Minh", cfg.CutoverTZ.String())
}

func TestLoad_cutoverOverrides__OVW_G7(t *testing.T) {
	cfg, err := Load(env(withLMK(map[string]string{envCutoverTime: "18:30:00", "CUTOVER_TZ": "UTC"})))

	require.NoError(t, err)
	require.Equal(t, 18*time.Hour+30*time.Minute, cfg.CutoverTime)
	require.Equal(t, "UTC", cfg.CutoverTZ.String())
}

func TestLoad_invalidCutoverFailsFast__OVW_G7(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"time not HH:MM:SS": {envCutoverTime: "23:59"},
		"time out of range": {envCutoverTime: "24:00:00"},
		"unknown zone":      {"CUTOVER_TZ": "Mars/Olympus_Mons"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(env(withLMK(m)))
			require.Error(t, err)
		})
	}
}
