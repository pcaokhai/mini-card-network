package isonet

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	toxiproxyAdminAddr = "http://localhost:8474"
	statusSignedOn     = "SIGNED_ON"
)

func toxiproxyReachable() bool {
	resp, err := http.Get(toxiproxyAdminAddr + "/proxies")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func setProxyEnabled(t *testing.T, name string, enabled bool) {
	t.Helper()
	body, err := json.Marshal(map[string]bool{"enabled": enabled})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, toxiproxyAdminAddr+"/proxies/"+name, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Less(t, resp.StatusCode, 300)
}

// TestSupervisor_recoversFromToxiproxyLinkDrop requires `make up` running first (real Toxiproxy,
// real issuer). It skips (not fails) otherwise, so `go test ./...` stays green without Docker.
func TestSupervisor_recoversFromToxiproxyLinkDrop__MCN_202_AC4(t *testing.T) {
	if !toxiproxyReachable() {
		t.Skip("Toxiproxy not reachable at localhost:8474 - run `make up` first for this integration test")
	}
	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: "localhost:18000", EchoInterval: time.Second, EchoFailureLimit: 3}, store)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { _ = sup.Run(ctx) }()

	require.Eventually(t, func() bool {
		return len(store.statuses) > 0 && store.statuses[len(store.statuses)-1] == statusSignedOn
	}, 10*time.Second, 50*time.Millisecond, "initial sign-on")

	setProxyEnabled(t, "issuer", false)
	t.Cleanup(func() { setProxyEnabled(t, "issuer", true) })

	require.Eventually(t, func() bool {
		return store.statuses[len(store.statuses)-1] == "DOWN"
	}, 10*time.Second, 50*time.Millisecond, "link goes down")

	setProxyEnabled(t, "issuer", true)

	require.Eventually(t, func() bool {
		return store.statuses[len(store.statuses)-1] == statusSignedOn
	}, 5*time.Second, 50*time.Millisecond, "AC4: recovers within 5s")
}
