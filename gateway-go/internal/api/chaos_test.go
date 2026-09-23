package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/chaos"
)

type fakeToxiproxyClient struct {
	scenarios []chaos.Scenario
	setErr    error
}

func (f *fakeToxiproxyClient) ListScenarios(context.Context) ([]chaos.Scenario, error) {
	return f.scenarios, nil
}

func (f *fakeToxiproxyClient) SetScenario(context.Context, chaos.ScenarioID, bool) error {
	return f.setErr
}

type fakeRunner struct {
	startResult chaos.ChaosRun
	getResult   *chaos.ChaosRun
}

func (f *fakeRunner) Start(context.Context, int) (*chaos.ChaosRun, error) { return &f.startResult, nil }

func (f *fakeRunner) Get(_ string) (*chaos.ChaosRun, bool) {
	if f.getResult == nil {
		return nil, false
	}
	return f.getResult, true
}

func TestGetChaosScenarios_returnsSixScenarios__MCN_404_AC1(t *testing.T) {
	scenarios := make([]chaos.Scenario, 0, len(chaos.AllScenarios))
	for _, id := range chaos.AllScenarios {
		scenarios = append(scenarios, chaos.Scenario{ID: id})
	}
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{scenarios: scenarios}, &fakeRunner{})

	req := httptest.NewRequest(http.MethodGet, "/v1/chaos/scenarios", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 6)
}

func TestPutChaosScenario_requiresIdempotencyKey__MCN_404_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{})

	req := httptest.NewRequest(http.MethodPut, "/v1/chaos/scenarios/SLOW_NETWORK", bytes.NewBufferString(`{"enabled":true}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutChaosScenario_dropResponseReturns501__MCN_404_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{setErr: chaos.ErrScenarioNotImplemented}, &fakeRunner{})

	req := httptest.NewRequest(http.MethodPut, "/v1/chaos/scenarios/DROP_RESPONSE", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Idempotency-Key", "t1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestPostChaosRuns_returns202WithRunningStatus__MCN_404_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{startResult: chaos.ChaosRun{RunID: "run-1", Status: "RUNNING"}})

	req := httptest.NewRequest(http.MethodPost, "/v1/chaos/runs", bytes.NewBufferString(`{"transactions":10}`))
	req.Header.Set("Idempotency-Key", "chaos-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	var got chaos.ChaosRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "RUNNING", got.Status)
}

func TestGetChaosRun_returns404WhenUnknown__MCN_404_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{})

	req := httptest.NewRequest(http.MethodGet, "/v1/chaos/runs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
