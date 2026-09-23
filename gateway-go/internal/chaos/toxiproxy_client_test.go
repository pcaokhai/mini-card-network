package chaos

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

const toxiproxyAdminAddr = "http://localhost:8474"

func toxiproxyReachable() bool {
	resp, err := http.Get(toxiproxyAdminAddr + "/proxies")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func TestToxiproxyClient_enableSlowNetworkAddsLatencyToxic__MCN_404_AC1(t *testing.T) {
	if !toxiproxyReachable() {
		t.Skip("Toxiproxy not reachable - run `make up` first")
	}
	client := NewToxiproxyClient(toxiproxyAdminAddr, "issuer")
	defer func() { _ = client.DisableAll(context.Background()) }()

	require.NoError(t, client.SetScenario(context.Background(), ScenarioSlowNetwork, true))

	states, err := client.ListScenarios(context.Background())
	require.NoError(t, err)
	require.True(t, findEnabled(states, ScenarioSlowNetwork))
}

func TestToxiproxyClient_disableAllClearsEverything__MCN_404_AC1(t *testing.T) {
	if !toxiproxyReachable() {
		t.Skip("Toxiproxy not reachable - run `make up` first")
	}
	client := NewToxiproxyClient(toxiproxyAdminAddr, "issuer")
	require.NoError(t, client.SetScenario(context.Background(), ScenarioConnectionCut, true))

	require.NoError(t, client.DisableAll(context.Background()))

	states, err := client.ListScenarios(context.Background())
	require.NoError(t, err)
	for _, s := range states {
		require.False(t, s.Enabled, "scenario %s should be disabled after DisableAll", s.ID)
	}
}

func findEnabled(states []Scenario, id ScenarioID) bool {
	for _, s := range states {
		if s.ID == id && s.Enabled {
			return true
		}
	}
	return false
}
