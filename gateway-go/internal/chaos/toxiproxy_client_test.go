package chaos

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const toxiproxyAdminAddr = "http://localhost:8474"

// toxiproxyReachable gates the tests that drive the real stack. They sign the lab acquirer on and
// off and toggle the issuer proxy, which signs the running gateway off at the issuer (every purchase
// then answers 91), so they run only when asked: MCN_STACK_TESTS=1 go test ./... with `make up`.
func toxiproxyReachable() bool {
	if os.Getenv("MCN_STACK_TESTS") != "1" {
		return false
	}
	resp, err := http.Get(toxiproxyAdminAddr + "/proxies")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func TestToxiproxyClient_enableSlowNetworkAddsLatencyToxic__MCN_404_AC1(t *testing.T) {
	if !toxiproxyReachable() {
		t.Skip("stack test: set MCN_STACK_TESTS=1 with `make up` running (it toggles the live issuer proxy)")
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
		t.Skip("stack test: set MCN_STACK_TESTS=1 with `make up` running (it toggles the live issuer proxy)")
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

func TestSetScenario_dropResponseNoLongerReturnsNotImplemented__MCN_407(t *testing.T) {
	if !toxiproxyReachable() {
		t.Skip("stack test: set MCN_STACK_TESTS=1 with `make up` running (it toggles the live issuer proxy)")
	}
	client := NewToxiproxyClient(toxiproxyAdminAddr, "issuer", WithDropResponseAddr("127.0.0.1:19999"))
	defer func() { _ = client.DisableAll(context.Background()) }()

	err := client.SetScenario(context.Background(), ScenarioDropResponse, true)

	require.NoError(t, err)
	require.NotErrorIs(t, err, ErrScenarioNotImplemented)
}

func TestSetScenario_dropResponseStillNotImplementedWhenUnwired__MCN_407(t *testing.T) {
	client := NewToxiproxyClient(toxiproxyAdminAddr, "issuer")

	err := client.SetScenario(context.Background(), ScenarioDropResponse, true)

	require.ErrorIs(t, err, ErrScenarioNotImplemented)
}

func findEnabled(states []Scenario, id ScenarioID) bool {
	for _, s := range states {
		if s.ID == id && s.Enabled {
			return true
		}
	}
	return false
}
