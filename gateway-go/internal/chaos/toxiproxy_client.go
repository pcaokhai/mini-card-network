package chaos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrScenarioNotImplemented is returned by SetScenario(DROP_RESPONSE, true) when no fake-issuer
// address is wired in (WithDropResponseAddr / CHAOS_FAKE_ISSUER_ADDR, MCN-407): production and
// every non-chaos environment leaves it unset, so this stays a deliberate stub, not a silent
// no-op, outside a chaos run.
var ErrScenarioNotImplemented = errors.New("chaos scenario not implemented yet")

const (
	streamDownstream = "downstream"
	toxicLatency     = "latency"
)

// toxicName is the Toxiproxy toxic name we register per scenario, so it can be found again to
// remove it. One toxic per scenario keeps toggling independent and idempotent.
func toxicName(id ScenarioID) string { return "chaos-" + string(id) }

// toxicSpec is what DisableAll/SetScenario(false) needs to know to add/remove the right toxic;
// DROP_RESPONSE has no Toxiproxy toxic (see the Ruling in docs/plans/MCN-404.md).
type toxicSpec struct {
	toxicType string
	stream    string
	attrs     map[string]any
}

func specFor(id ScenarioID) (toxicSpec, bool) {
	switch id {
	case ScenarioSlowNetwork:
		return toxicSpec{toxicType: toxicLatency, stream: streamDownstream, attrs: map[string]any{toxicLatency: 3000, "jitter": 0}}, true
	case ScenarioConnectionCut, ScenarioIssuerDown:
		return toxicSpec{toxicType: "reset_peer", stream: streamDownstream, attrs: map[string]any{"timeout": 0}}, true
	case ScenarioLateResponse:
		// docs/03 §9: 30s request timeout - exceed it so the response arrives late (MCN-403's path).
		return toxicSpec{toxicType: toxicLatency, stream: streamDownstream, attrs: map[string]any{toxicLatency: 35000, "jitter": 0}}, true
	case ScenarioDuplicateRequest:
		// Not a Toxiproxy toxic; implemented gateway-side via purchase.Service's duplicate hook.
		return toxicSpec{}, false
	case ScenarioDropResponse:
		// No fake-issuer response-dropping mode yet (MCN-407, Sprint 6).
		return toxicSpec{}, false
	default:
		return toxicSpec{}, false
	}
}

// ToxiproxyClient toggles chaos scenarios against a real Toxiproxy proxy's admin API
// (infra/toxiproxy/toxiproxy.json's "issuer" proxy).
type ToxiproxyClient struct {
	adminAddr string
	proxyName string
	http      *http.Client

	// dropResponseAddr is the fake-issuer simulator's address (CHAOS_FAKE_ISSUER_ADDR). Empty
	// outside a chaos run, in which case DROP_RESPONSE keeps returning ErrScenarioNotImplemented
	// so an un-wired gateway fails loud instead of silently no-op'ing.
	dropResponseAddr string
	realIssuerAddr   string // remembered so SetScenario(false) can restore it; set on first enable

	// enabled tracks scenarios with no Toxiproxy-side toxic (DUPLICATE_REQUEST, DROP_RESPONSE)
	// so ListScenarios reports their state too. Never mutated concurrently with itself from
	// more than one goroutine in practice (single HTTP handler at a time serializes writes),
	// but guarded because ListScenarios can race a concurrent SetScenario in tests.
	localState map[ScenarioID]bool
}

// ToxiproxyClientOption configures optional ToxiproxyClient behavior.
type ToxiproxyClientOption func(*ToxiproxyClient)

// WithDropResponseAddr wires DROP_RESPONSE to repoint the issuer proxy at a fake-issuer
// simulator's address instead of returning ErrScenarioNotImplemented.
func WithDropResponseAddr(addr string) ToxiproxyClientOption {
	return func(c *ToxiproxyClient) { c.dropResponseAddr = addr }
}

// NewToxiproxyClient builds a client against adminAddr (e.g. "http://localhost:8474") for the
// named proxy.
func NewToxiproxyClient(adminAddr, proxyName string, opts ...ToxiproxyClientOption) *ToxiproxyClient {
	c := &ToxiproxyClient{adminAddr: adminAddr, proxyName: proxyName, http: http.DefaultClient, localState: map[ScenarioID]bool{}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetScenario enables or disables id. For scenarios backed by a real Toxiproxy toxic, it
// adds/removes that toxic; for gateway-side-only scenarios it just records local state.
func (c *ToxiproxyClient) SetScenario(ctx context.Context, id ScenarioID, enabled bool) error {
	if id == ScenarioDropResponse {
		if c.dropResponseAddr == "" {
			if enabled {
				return ErrScenarioNotImplemented
			}
			c.localState[id] = false
			return nil
		}
		if err := c.setDropResponse(ctx, enabled); err != nil {
			return err
		}
		c.localState[id] = enabled
		return nil
	}
	spec, hasToxic := specFor(id)
	if !hasToxic {
		c.localState[id] = enabled
		return nil
	}
	if enabled {
		return c.addToxic(ctx, id, spec)
	}
	return c.removeToxic(ctx, id)
}

// DisableAll removes every chaos toxic and clears local state - called once at gateway boot so a
// crashed prior run can't leave chaos permanently injected.
func (c *ToxiproxyClient) DisableAll(ctx context.Context) error {
	for _, id := range AllScenarios {
		if err := c.SetScenario(ctx, id, false); err != nil {
			return fmt.Errorf("disable %s: %w", id, err)
		}
	}
	return nil
}

// ListScenarios reports current enabled state for all six scenarios.
func (c *ToxiproxyClient) ListScenarios(ctx context.Context) ([]Scenario, error) {
	active, err := c.activeToxicNames(ctx)
	if err != nil {
		return nil, err
	}
	scenarios := make([]Scenario, 0, len(AllScenarios))
	for _, id := range AllScenarios {
		text := scenarioText[id]
		_, hasToxic := specFor(id)
		enabled := c.localState[id]
		if hasToxic {
			enabled = active[toxicName(id)]
		}
		scenarios = append(scenarios, Scenario{ID: id, Enabled: enabled, EasyText: text[0], TechnicalText: text[1]})
	}
	return scenarios, nil
}

func (c *ToxiproxyClient) addToxic(ctx context.Context, id ScenarioID, spec toxicSpec) error {
	body, err := json.Marshal(map[string]any{
		"name": toxicName(id), "type": spec.toxicType, "stream": spec.stream, "attributes": spec.attrs,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.toxicsURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("add toxic %s: %w", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Toxiproxy returns 409 if the toxic already exists (SetScenario(true) called twice); treat
	// as success since the desired state (toxic present) already holds.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add toxic %s: status %d: %s", id, resp.StatusCode, msg)
	}
	return nil
}

func (c *ToxiproxyClient) removeToxic(ctx context.Context, id ScenarioID) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.toxicsURL()+"/"+toxicName(id), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("remove toxic %s: %w", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// 404 means it's already gone - that's the desired state, not an error.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove toxic %s: status %d: %s", id, resp.StatusCode, msg)
	}
	return nil
}

func (c *ToxiproxyClient) activeToxicNames(ctx context.Context) (map[string]bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.toxicsURL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list toxics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list toxics: status %d: %s", resp.StatusCode, msg)
	}
	var toxics []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&toxics); err != nil {
		return nil, fmt.Errorf("decode toxics: %w", err)
	}
	active := make(map[string]bool, len(toxics))
	for _, t := range toxics {
		active[t.Name] = true
	}
	return active, nil
}

func (c *ToxiproxyClient) toxicsURL() string {
	return c.adminAddr + "/proxies/" + c.proxyName + "/toxics"
}

// setDropResponse repoints the issuer proxy's upstream at the fake-issuer simulator (enabled) or
// restores the real issuer upstream it remembered on first enable (disabled).
func (c *ToxiproxyClient) setDropResponse(ctx context.Context, enabled bool) error {
	if !enabled {
		if c.realIssuerAddr == "" {
			return nil // never enabled - nothing to restore
		}
		return c.updateUpstream(ctx, c.realIssuerAddr)
	}
	if c.realIssuerAddr == "" {
		current, err := c.currentUpstream(ctx)
		if err != nil {
			return err
		}
		c.realIssuerAddr = current
	}
	return c.updateUpstream(ctx, c.dropResponseAddr)
}

func (c *ToxiproxyClient) proxyURL() string {
	return c.adminAddr + "/proxies/" + c.proxyName
}

func (c *ToxiproxyClient) currentUpstream(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.proxyURL(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("get proxy %s: %w", c.proxyName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get proxy %s: status %d: %s", c.proxyName, resp.StatusCode, msg)
	}
	var proxy struct {
		Upstream string `json:"upstream"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&proxy); err != nil {
		return "", fmt.Errorf("decode proxy %s: %w", c.proxyName, err)
	}
	return proxy.Upstream, nil
}

func (c *ToxiproxyClient) updateUpstream(ctx context.Context, upstream string) error {
	body, err := json.Marshal(map[string]any{"upstream": upstream})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.proxyURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("update proxy %s upstream: %w", c.proxyName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update proxy %s upstream: status %d: %s", c.proxyName, resp.StatusCode, msg)
	}
	return nil
}
