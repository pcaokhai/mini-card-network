package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
	List(limit int) []chaos.ChaosRun
}

// ChaosHub broadcasts chaos.changed after a scenario toggle. *ws.Hub satisfies it.
type ChaosHub interface {
	BroadcastChaos(eventType string, data any)
}

const (
	maxChaosTransactions = 10000 // contracts/openapi.yaml startChaosRun maximum
	defaultChaosRunLimit = 50    // components.parameters.Limit
	maxChaosRunLimit     = 200

	slugValidation     = "validation-error"
	slugNotFound       = "not-found"
	slugInternal       = "internal"
	slugIdempotencyKey = "insufficient-idempotency-key"
)

// MountChaos registers the chaos routes (contracts/openapi.yaml, tag "chaos"). hub may be nil in
// tests that don't assert on WS broadcasts.
func MountChaos(r chi.Router, toxiproxy ToxiproxyPort, runner RunnerPort, hub ...ChaosHub) {
	var h ChaosHub
	if len(hub) > 0 {
		h = hub[0]
	}
	r.Get("/v1/chaos/scenarios", handleListChaosScenarios(toxiproxy))
	r.Put("/v1/chaos/scenarios/{scenarioId}", handleSetChaosScenario(toxiproxy, h))
	r.Get("/v1/chaos/runs", handleListChaosRuns(runner))
	r.Post("/v1/chaos/runs", handleStartChaosRun(runner, &runReplays{byKey: map[string]runReplay{}}))
	r.Get("/v1/chaos/runs/{runId}", handleGetChaosRun(runner))
}

// hasUUIDIdempotencyKey enforces docs/04 §2: every state-changing call carries a UUID key.
func hasUUIDIdempotencyKey(w http.ResponseWriter, req *http.Request) bool {
	if uuid.Validate(req.Header.Get("Idempotency-Key")) != nil {
		problem(w, http.StatusBadRequest, slugIdempotencyKey, "Idempotency-Key header must be a UUID")
		return false
	}
	return true
}

func knownScenario(id chaos.ScenarioID) bool {
	return slices.Contains(chaos.AllScenarios, id)
}

func handleListChaosScenarios(toxiproxy ToxiproxyPort) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		scenarios, err := toxiproxy.ListScenarios(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, slugInternal, "could not read chaos scenario state")
			return
		}
		writeJSONBody(w, http.StatusOK, scenarios)
	}
}

func handleSetChaosScenario(toxiproxy ToxiproxyPort, hub ChaosHub) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !hasUUIDIdempotencyKey(w, req) {
			return
		}
		id := chaos.ScenarioID(chi.URLParam(req, "scenarioId"))
		if !knownScenario(id) {
			problem(w, http.StatusNotFound, slugNotFound, "no such chaos scenario")
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Enabled == nil {
			problem(w, http.StatusBadRequest, slugValidation, "body must be {\"enabled\": true|false}")
			return
		}
		if err := toxiproxy.SetScenario(req.Context(), id, *body.Enabled); err != nil {
			if errors.Is(err, chaos.ErrScenarioNotImplemented) {
				problem(w, http.StatusNotImplemented, "scenario-unavailable", "this stack cannot run "+string(id)+" (no fake issuer configured)")
				return
			}
			problem(w, http.StatusInternalServerError, slugInternal, "could not apply the chaos scenario")
			return
		}
		updated, err := scenarioState(req.Context(), toxiproxy, id)
		if err != nil {
			problem(w, http.StatusInternalServerError, slugInternal, "could not read chaos scenario state")
			return
		}
		if hub != nil {
			hub.BroadcastChaos("chaos.changed", updated)
		}
		writeJSONBody(w, http.StatusOK, updated)
	}
}

func scenarioState(ctx context.Context, toxiproxy ToxiproxyPort, id chaos.ScenarioID) (chaos.Scenario, error) {
	scenarios, err := toxiproxy.ListScenarios(ctx)
	if err != nil {
		return chaos.Scenario{}, err
	}
	for _, s := range scenarios {
		if s.ID == id {
			return s, nil
		}
	}
	return chaos.Scenario{ID: id}, nil
}

// runReplays remembers each Idempotency-Key's run so a retried POST gets the same run back
// instead of starting a second one (docs/04 §2).
// ponytail: in memory and never expired, like the runs themselves; bounded by how many runs one
// gateway process starts. Persist with the runs if they ever outlive a restart.
type runReplays struct {
	mu    sync.Mutex
	byKey map[string]runReplay
}

type runReplay struct {
	transactions int
	run          chaos.ChaosRun
}

func handleStartChaosRun(runner RunnerPort, replays *runReplays) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !hasUUIDIdempotencyKey(w, req) {
			return
		}
		var body struct {
			Transactions int `json:"transactions"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Transactions < 1 || body.Transactions > maxChaosTransactions {
			problem(w, http.StatusBadRequest, slugValidation, "transactions must be an integer from 1 to 10000")
			return
		}
		key := req.Header.Get("Idempotency-Key")
		// Held across Start so two concurrent retries of one key can't both start a run.
		replays.mu.Lock()
		defer replays.mu.Unlock()
		if prior, ok := replays.byKey[key]; ok {
			if prior.transactions != body.Transactions {
				problem(w, http.StatusUnprocessableEntity, "idempotency-key-mismatch", "Idempotency-Key was already used with a different request")
				return
			}
			writeJSONBody(w, http.StatusAccepted, prior.run)
			return
		}
		run, err := runner.Start(req.Context(), body.Transactions)
		if errors.Is(err, chaos.ErrRunInProgress) {
			problem(w, http.StatusConflict, "conflict", "a chaos run is already in progress")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, slugInternal, "could not start the chaos run")
			return
		}
		replays.byKey[key] = runReplay{transactions: body.Transactions, run: *run}
		writeJSONBody(w, http.StatusAccepted, run)
	}
}

func handleListChaosRuns(runner RunnerPort) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		limit := defaultChaosRunLimit
		if raw := req.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > maxChaosRunLimit {
				problem(w, http.StatusBadRequest, slugValidation, "limit must be an integer from 1 to 200")
				return
			}
			limit = n
		}
		writeJSONBody(w, http.StatusOK, map[string]any{"items": runner.List(limit)})
	}
}

func handleGetChaosRun(runner RunnerPort) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		run, ok := runner.Get(chi.URLParam(req, "runId"))
		if !ok {
			problem(w, http.StatusNotFound, slugNotFound, "no such chaos run")
			return
		}
		writeJSONBody(w, http.StatusOK, run)
	}
}
