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
	startErr    error
	starts      int
	getResult   *chaos.ChaosRun
	listed      []chaos.ChaosRun
	listLimit   int
}

func (f *fakeRunner) Start(context.Context, int) (*chaos.ChaosRun, error) {
	f.starts++
	if f.startErr != nil {
		return nil, f.startErr
	}
	run := f.startResult
	return &run, nil
}

func (f *fakeRunner) List(limit int) []chaos.ChaosRun {
	f.listLimit = limit
	return f.listed
}

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
	req.Header.Set("Idempotency-Key", testKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestPostChaosRuns_returns202WithRunningStatus__MCN_404_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{startResult: chaos.ChaosRun{RunID: testRunID, Status: runRunning}})

	req := httptest.NewRequest(http.MethodPost, "/v1/chaos/runs", bytes.NewBufferString(`{"transactions":10}`))
	req.Header.Set("Idempotency-Key", testKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	var got chaos.ChaosRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, runRunning, got.Status)
}

func TestGetChaosRun_returns404WhenUnknown__MCN_404_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{})

	req := httptest.NewRequest(http.MethodGet, "/v1/chaos/runs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

const (
	testKey    = "11111111-1111-1111-1111-111111111111"
	runRunning = "RUNNING"
	testRunID  = "run-1"
)

func chaosRequest(t *testing.T, r http.Handler, method, path, key, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	return rec.Code, got
}

func TestPutChaosScenario_validatesIdAndEnabled__CHA_G4(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{})

	code, got := chaosRequest(t, r, http.MethodPut, "/v1/chaos/scenarios/NOT_A_SCENARIO", testKey, `{"enabled":true}`)
	require.Equal(t, http.StatusNotFound, code)
	require.Equal(t, problemTypeBase+"not-found", got["type"])

	for _, body := range []string{`{}`, `{"enabled":null}`, `not json`} {
		code, got = chaosRequest(t, r, http.MethodPut, "/v1/chaos/scenarios/SLOW_NETWORK", testKey, body)
		require.Equal(t, http.StatusBadRequest, code, body)
		require.Equal(t, problemTypeBase+"validation-error", got["type"], body)
	}
}

func TestPutChaosScenario_dropResponseWithoutFakeIssuerIsScenarioUnavailable__CHA_G11(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{setErr: chaos.ErrScenarioNotImplemented}, &fakeRunner{})

	code, got := chaosRequest(t, r, http.MethodPut, "/v1/chaos/scenarios/DROP_RESPONSE", testKey, `{"enabled":true}`)
	require.Equal(t, http.StatusNotImplemented, code)
	require.Equal(t, problemTypeBase+"scenario-unavailable", got["type"])
}

func TestChaosWrites_requireAUUIDIdempotencyKey__CHA_G5_G12(t *testing.T) {
	r := chi.NewRouter()
	runner := &fakeRunner{}
	MountChaos(r, &fakeToxiproxyClient{}, runner)

	for _, key := range []string{"", "chaos-1"} {
		code, got := chaosRequest(t, r, http.MethodPost, "/v1/chaos/runs", key, `{"transactions":10}`)
		require.Equal(t, http.StatusBadRequest, code)
		require.Equal(t, problemTypeBase+"insufficient-idempotency-key", got["type"])
		code, got = chaosRequest(t, r, http.MethodPut, "/v1/chaos/scenarios/SLOW_NETWORK", key, `{"enabled":true}`)
		require.Equal(t, http.StatusBadRequest, code)
		require.Equal(t, problemTypeBase+"insufficient-idempotency-key", got["type"])
	}
	require.Zero(t, runner.starts)
}

func TestPostChaosRuns_boundsTransactions__CHA_G5(t *testing.T) {
	r := chi.NewRouter()
	runner := &fakeRunner{}
	MountChaos(r, &fakeToxiproxyClient{}, runner)

	for _, body := range []string{`{"transactions":0}`, `{"transactions":10001}`, `{}`, `nope`} {
		code, got := chaosRequest(t, r, http.MethodPost, "/v1/chaos/runs", testKey, body)
		require.Equal(t, http.StatusBadRequest, code, body)
		require.Equal(t, problemTypeBase+"validation-error", got["type"], body)
	}
	require.Zero(t, runner.starts)
}

func TestPostChaosRuns_replaysTheSameRunForTheSameKey__CHA_G5(t *testing.T) {
	r := chi.NewRouter()
	runner := &fakeRunner{startResult: chaos.ChaosRun{RunID: testRunID, Status: runRunning}}
	MountChaos(r, &fakeToxiproxyClient{}, runner)

	code, first := chaosRequest(t, r, http.MethodPost, "/v1/chaos/runs", testKey, `{"transactions":10}`)
	require.Equal(t, http.StatusAccepted, code)
	code, replay := chaosRequest(t, r, http.MethodPost, "/v1/chaos/runs", testKey, `{"transactions":10}`)
	require.Equal(t, http.StatusAccepted, code)
	require.Equal(t, first["runId"], replay["runId"])
	require.Equal(t, 1, runner.starts)

	code, got := chaosRequest(t, r, http.MethodPost, "/v1/chaos/runs", testKey, `{"transactions":20}`)
	require.Equal(t, http.StatusUnprocessableEntity, code)
	require.Equal(t, problemTypeBase+"idempotency-key-mismatch", got["type"])
	require.Equal(t, 1, runner.starts)
}

func TestPostChaosRuns_conflictWhileARunIsInProgress__CHA_G5(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{startErr: chaos.ErrRunInProgress})

	code, got := chaosRequest(t, r, http.MethodPost, "/v1/chaos/runs", testKey, `{"transactions":10}`)
	require.Equal(t, http.StatusConflict, code)
	require.Equal(t, problemTypeBase+"conflict", got["type"])
}

func TestGetChaosRuns_listsNewestFirstWithLimit__CHA_G6(t *testing.T) {
	r := chi.NewRouter()
	runner := &fakeRunner{listed: []chaos.ChaosRun{{RunID: "run-2"}, {RunID: testRunID}}}
	MountChaos(r, &fakeToxiproxyClient{}, runner)

	code, got := chaosRequest(t, r, http.MethodGet, "/v1/chaos/runs?limit=1", "", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 1, runner.listLimit)
	require.Len(t, got["items"], 2)

	_, _ = chaosRequest(t, r, http.MethodGet, "/v1/chaos/runs", "", "")
	require.Equal(t, 50, runner.listLimit)

	for _, bad := range []string{"0", "201", "x"} {
		code, got = chaosRequest(t, r, http.MethodGet, "/v1/chaos/runs?limit="+bad, "", "")
		require.Equal(t, http.StatusBadRequest, code, bad)
		require.Equal(t, problemTypeBase+"validation-error", got["type"], bad)
	}
}

func TestGetChaosRun_unknownIsNotFoundSlug__CHA_G12(t *testing.T) {
	r := chi.NewRouter()
	MountChaos(r, &fakeToxiproxyClient{}, &fakeRunner{})

	code, got := chaosRequest(t, r, http.MethodGet, "/v1/chaos/runs/nope", "", "")
	require.Equal(t, http.StatusNotFound, code)
	require.Equal(t, problemTypeBase+"not-found", got["type"])
}
