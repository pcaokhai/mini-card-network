package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCORSMiddleware_emptyOriginIsPassthrough__R10(t *testing.T) {
	called := false
	h := CORSMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil))

	require.True(t, called)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_setsHeadersAndHandlesPreflight__R10(t *testing.T) {
	called := false
	h := CORSMiddleware("http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil))
	require.True(t, called)
	require.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))

	called = false
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/v1/metrics/overview", nil))
	require.False(t, called, "OPTIONS preflight must not reach the wrapped handler")
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
	require.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Headers"))
}
