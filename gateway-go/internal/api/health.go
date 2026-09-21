// Package api exposes the gateway's HTTP interface.
package api

import (
	"net/http"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Health tracks readiness; it becomes not-ready when graceful shutdown starts.
type Health struct{ draining atomic.Bool }

// NewHealth returns a ready Health.
func NewHealth() *Health { return &Health{} }

// StartDraining makes /health/ready fail so traffic stops before the server shuts down.
func (h *Health) StartDraining() { h.draining.Store(true) }

// NewRouter builds the HTTP handler; every request gets a server span (W3C traceparent honored).
func NewRouter(health *Health) http.Handler {
	r := chi.NewRouter()
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"status":"UP"}`)
	})
	r.Get("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if health.draining.Load() {
			writeJSON(w, http.StatusServiceUnavailable, `{"status":"DRAINING"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"UP"}`)
	})
	return otelhttp.NewHandler(r, "gateway-http")
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
