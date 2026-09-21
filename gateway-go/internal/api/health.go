// Package api exposes the gateway's HTTP interface.
package api

import (
	"net/http"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
)

// Health tracks readiness; it becomes not-ready when graceful shutdown starts.
type Health struct{ draining atomic.Bool }

// NewHealth returns a ready Health.
func NewHealth() *Health { return &Health{} }

// StartDraining makes /health/ready fail so traffic stops before the server shuts down.
func (h *Health) StartDraining() { h.draining.Store(true) }

// NewRouter registers the health routes on r; call MountLab(r) separately to add the Lab API.
// Kept as a function taking chi.Router (not creating its own) so cmd/gateway can compose routers
// under one otelhttp span wrapper.
func NewRouter(r chi.Router, health *Health) {
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
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
