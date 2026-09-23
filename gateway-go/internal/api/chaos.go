package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/chaos"
)

// ToxiproxyPort is the port chaos.go needs to toggle scenarios; *chaos.ToxiproxyClient
// satisfies it.
type ToxiproxyPort interface {
	ListScenarios(ctx context.Context) ([]chaos.Scenario, error)
	SetScenario(ctx context.Context, id chaos.ScenarioID, enabled bool) error
}

// RunnerPort is the port chaos.go needs to start and inspect chaos runs; *chaos.Runner
// satisfies it.
type RunnerPort interface {
	Start(ctx context.Context, requested int) (*chaos.ChaosRun, error)
	Get(runID string) (*chaos.ChaosRun, bool)
}

// ChaosHub broadcasts chaos.changed after a scenario toggle. *ws.Hub satisfies it.
type ChaosHub interface {
	BroadcastChaos(eventType string, data any)
}

// MountChaos registers the chaos routes (contracts/openapi.yaml, tag "chaos"). hub may be nil in
// tests that don't assert on WS broadcasts.
func MountChaos(r chi.Router, toxiproxy ToxiproxyPort, runner RunnerPort, hub ...ChaosHub) {
	var h ChaosHub
	if len(hub) > 0 {
		h = hub[0]
	}
	r.Get("/v1/chaos/scenarios", handleListChaosScenarios(toxiproxy))
	r.Put("/v1/chaos/scenarios/{scenarioId}", handleSetChaosScenario(toxiproxy, h))
	r.Post("/v1/chaos/runs", handleStartChaosRun(runner))
	r.Get("/v1/chaos/runs/{runId}", handleGetChaosRun(runner))
}

func handleListChaosScenarios(toxiproxy ToxiproxyPort) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		scenarios, err := toxiproxy.ListScenarios(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, "chaos-list-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, scenarios)
	}
}

func handleSetChaosScenario(toxiproxy ToxiproxyPort, hub ChaosHub) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Idempotency-Key") == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		id := chaos.ScenarioID(chi.URLParam(req, "scenarioId"))
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		if err := toxiproxy.SetScenario(req.Context(), id, body.Enabled); err != nil {
			if errors.Is(err, chaos.ErrScenarioNotImplemented) {
				problem(w, http.StatusNotImplemented, "scenario-not-implemented", err.Error())
				return
			}
			problem(w, http.StatusInternalServerError, "chaos-set-failed", err.Error())
			return
		}
		scenarios, err := toxiproxy.ListScenarios(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, "chaos-list-failed", err.Error())
			return
		}
		var updated chaos.Scenario
		for _, s := range scenarios {
			if s.ID == id {
				updated = s
			}
		}
		if hub != nil {
			hub.BroadcastChaos("chaos.changed", updated)
		}
		writeJSONBody(w, http.StatusOK, updated)
	}
}

func handleStartChaosRun(runner RunnerPort) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Idempotency-Key") == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body struct {
			Transactions int `json:"transactions"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		if body.Transactions < 1 {
			problem(w, http.StatusBadRequest, "invalid-request", "transactions must be at least 1")
			return
		}
		run, err := runner.Start(req.Context(), body.Transactions)
		if err != nil {
			problem(w, http.StatusInternalServerError, "chaos-run-start-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusAccepted, run)
	}
}

func handleGetChaosRun(runner RunnerPort) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		run, ok := runner.Get(chi.URLParam(req, "runId"))
		if !ok {
			problem(w, http.StatusNotFound, "chaos-run-not-found", "no such chaos run")
			return
		}
		writeJSONBody(w, http.StatusOK, run)
	}
}
