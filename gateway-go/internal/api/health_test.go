package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealth__MCN_005_AC1(t *testing.T) {
	health := NewHealth()
	router := NewRouter(health)

	require.Equal(t, http.StatusOK, get(t, router, "/health/live").Code)
	ready := get(t, router, "/health/ready")
	require.Equal(t, http.StatusOK, ready.Code)
	require.JSONEq(t, `{"status":"UP"}`, ready.Body.String())
}

func TestHealth_readyTurnsDownWhileDraining__MCN_005_AC4(t *testing.T) {
	health := NewHealth()
	router := NewRouter(health)

	health.StartDraining()

	require.Equal(t, http.StatusOK, get(t, router, "/health/live").Code)
	ready := get(t, router, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, ready.Code)
	require.JSONEq(t, `{"status":"DRAINING"}`, ready.Body.String())
}
