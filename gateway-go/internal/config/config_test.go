package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_defaults__MCN_005_AC1(t *testing.T) {
	cfg, err := Load(env(nil))
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, ":9464", cfg.MetricsAddr)
	require.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	require.False(t, cfg.TracingEnabled)
}

func TestLoad_overrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"HTTP_ADDR": ":18080", "SHUTDOWN_TIMEOUT": "5s", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4317",
	}))
	require.NoError(t, err)
	require.Equal(t, ":18080", cfg.HTTPAddr)
	require.Equal(t, 5*time.Second, cfg.ShutdownTimeout)
	require.True(t, cfg.TracingEnabled)
}

func TestLoad_rejectsInvalidValues(t *testing.T) {
	_, err := Load(env(map[string]string{"SHUTDOWN_TIMEOUT": "soon"}))
	require.ErrorContains(t, err, "SHUTDOWN_TIMEOUT")
	_, err = Load(env(map[string]string{"SHUTDOWN_TIMEOUT": "0s"}))
	require.ErrorContains(t, err, "must be positive")
}
