package ws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

func TestHub_broadcastsLinkStatusToConnectedClients__MCN_204_AC4(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(hub)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	defer func() { _ = resp.Body.Close() }()

	require.Eventually(t, func() bool { return hub.ClientCount() == 1 }, time.Second, 10*time.Millisecond)

	hub.BroadcastLinkStatus(store.Link{Endpoint: "issuer", Status: "SIGNED_ON"})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var evt Event
	require.NoError(t, json.Unmarshal(msg, &evt))
	require.Equal(t, "link.status", evt.Type)
	require.Contains(t, string(msg), `"status":"SIGNED_ON"`)
}

func TestHub_broadcastsNetworkEventToConnectedClients__MCN_204_AC4(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(hub)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	defer func() { _ = resp.Body.Close() }()

	require.Eventually(t, func() bool { return hub.ClientCount() == 1 }, time.Second, 10*time.Millisecond)

	hub.BroadcastNetworkEvent(store.NetworkEvent{Severity: "INFO", EasyText: "Link is healthy", TechnicalText: "issuer SIGNED_ON"})

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(msg), `"type":"network.event"`)
	require.Contains(t, string(msg), "Link is healthy")
}
