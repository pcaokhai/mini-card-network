// Command chaos drives the six MCN-404 chaos scenarios against a running gateway (make up),
// verifies the ledger invariant and SAF drain, and writes chaos-report.md (MCN-407).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// scenarioIDs mirrors gateway-go's internal/chaos.AllScenarios order - kept in sync manually
// since this CLI is a separate module with no dependency on gateway-go's internal packages.
var scenarioIDs = []string{
	"SLOW_NETWORK",
	"CONNECTION_CUT",
	"DROP_RESPONSE",
	"DUPLICATE_REQUEST",
	"ISSUER_DOWN",
	"LATE_RESPONSE",
}

const (
	defaultGatewayURL       = "http://localhost:8080"
	defaultTxPerScenario    = 500
	nightlyTxPerScenario    = 10000
	pollInterval            = 2 * time.Second
	runTimeoutSmall         = 90 * time.Second
	runTimeoutLarge         = 20 * time.Minute
	idempotencyKeyScenario  = "chaos-cli-scenario"
	idempotencyKeyRunPrefix = "chaos-cli-run-"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "chaos suite:", err)
		os.Exit(1)
	}
}

func run() error {
	gatewayURL := envOr("GATEWAY_URL", defaultGatewayURL)
	txPerScenario := envIntOr("CHAOS_TX_PER_SCENARIO", defaultTxPerScenario)
	runTimeout := runTimeoutSmall
	if txPerScenario >= nightlyTxPerScenario {
		runTimeout = runTimeoutLarge
	}
	if override := os.Getenv("CHAOS_RUN_TIMEOUT"); override != "" {
		d, err := time.ParseDuration(override)
		if err != nil {
			return fmt.Errorf("CHAOS_RUN_TIMEOUT: %w", err)
		}
		runTimeout = d
	}

	client := &httpClient{base: gatewayURL, http: http.DefaultClient}

	results := make([]ScenarioResult, 0, len(scenarioIDs))
	for _, id := range scenarioIDs {
		result, err := runScenario(client, id, txPerScenario, runTimeout)
		if err != nil {
			return fmt.Errorf("scenario %s: %w", id, err)
		}
		results = append(results, result)
	}

	safDeadCount, err := client.safDeadCount()
	if err != nil {
		return fmt.Errorf("read saf status: %w", err)
	}

	report := RenderReport(results, safDeadCount)
	if err := os.WriteFile("chaos-report.md", []byte(report), 0o644); err != nil { //nolint:gosec // report is not sensitive
		return fmt.Errorf("write chaos-report.md: %w", err)
	}
	fmt.Print(report)

	if ReportFailed(results, safDeadCount) {
		os.Exit(1)
	}
	return nil
}

func runScenario(client *httpClient, id string, requested int, timeout time.Duration) (ScenarioResult, error) {
	if err := client.setScenario(id, true); err != nil {
		return ScenarioResult{}, fmt.Errorf("enable: %w", err)
	}
	defer func() { _ = client.setScenario(id, false) }()

	run, err := client.startRun(id, requested)
	if err != nil {
		return ScenarioResult{}, fmt.Errorf("start run: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for run.Status == "RUNNING" {
		if time.Now().After(deadline) {
			return ScenarioResult{}, fmt.Errorf("run %s timed out after %s", run.RunID, timeout)
		}
		time.Sleep(pollInterval)
		run, err = client.getRun(run.RunID)
		if err != nil {
			return ScenarioResult{}, fmt.Errorf("poll run: %w", err)
		}
	}

	return ScenarioResult{
		ID: id, Requested: run.Requested, Approved: run.Approved, Declined: run.Declined,
		Reversed: run.Reversed, LedgerDiscrepancy: run.LedgerDiscrepancy, Status: run.Status,
	}, nil
}

// chaosRun mirrors gateway-go's internal/chaos.ChaosRun (contracts/openapi.yaml's ChaosRun schema).
type chaosRun struct {
	RunID             string `json:"runId"`
	Status            string `json:"status"`
	Requested         int    `json:"requested"`
	Approved          int    `json:"approved"`
	Declined          int    `json:"declined"`
	Reversed          int    `json:"reversed"`
	LedgerDiscrepancy int64  `json:"ledgerDiscrepancy"`
}

type httpClient struct {
	base string
	http *http.Client
}

func (c *httpClient) setScenario(id string, enabled bool) error {
	body, err := json.Marshal(map[string]any{"enabled": enabled})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, c.base+"/v1/chaos/scenarios/"+id, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKeyScenario+"-"+id+"-"+strconv.FormatBool(enabled))
	return c.doExpectOK(req, nil)
}

func (c *httpClient) startRun(scenarioID string, requested int) (chaosRun, error) {
	body, err := json.Marshal(map[string]any{"transactions": requested})
	if err != nil {
		return chaosRun{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/v1/chaos/runs", bytes.NewReader(body))
	if err != nil {
		return chaosRun{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKeyRunPrefix+scenarioID)
	var run chaosRun
	if err := c.doExpectOK(req, &run); err != nil {
		return chaosRun{}, err
	}
	return run, nil
}

func (c *httpClient) getRun(runID string) (chaosRun, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+"/v1/chaos/runs/"+runID, nil)
	if err != nil {
		return chaosRun{}, err
	}
	var run chaosRun
	if err := c.doExpectOK(req, &run); err != nil {
		return chaosRun{}, err
	}
	return run, nil
}

func (c *httpClient) safDeadCount() (int, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+"/v1/network/saf", nil)
	if err != nil {
		return 0, err
	}
	var body struct {
		DeadCount int `json:"deadCount"`
	}
	if err := c.doExpectOK(req, &body); err != nil {
		return 0, err
	}
	return body.DeadCount, nil
}

func (c *httpClient) doExpectOK(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
