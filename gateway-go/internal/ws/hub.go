// Package ws implements the gateway's /v1/stream WebSocket broadcast hub
// (contracts/ws-events.schema.json). It has no notion of ISO 8583 or the
// domain; it only fans out already-shaped Event envelopes to every connected client.
package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

// Event is the wire envelope from contracts/ws-events.schema.json.
type Event struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Data       any       `json:"data"`
}

// Hub fans out Events to every connected WebSocket client.
type Hub struct {
	upgrader websocket.Upgrader
	mu       sync.Mutex
	clients  map[chan []byte]struct{}
}

// NewHub builds an empty Hub. Its ServeHTTP upgrades any request to a WebSocket
// client that receives every subsequent broadcast.
func NewHub() *Hub {
	return &Hub{
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		clients:  map[chan []byte]struct{}{},
	}
}

// ServeHTTP upgrades the request to a WebSocket and streams broadcasts to it until it disconnects.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	out := h.subscribe()
	defer h.unsubscribe(out)

	// Drain client-initiated frames (pings/close) so the connection's read side never blocks writes.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for msg := range out {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// ClientCount reports how many clients are currently connected. It exists for tests.
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// BroadcastLinkStatus sends a link.status event carrying the given link's current state.
func (h *Hub) BroadcastLinkStatus(link store.Link) { h.broadcastEvent("link.status", link) }

// BroadcastNetworkEvent sends a network.event event carrying evt.
func (h *Hub) BroadcastNetworkEvent(evt store.NetworkEvent) { h.broadcastEvent("network.event", evt) }

// BroadcastTransaction sends a transaction event (eventType, e.g. "transaction.created")
// carrying txn.
func (h *Hub) BroadcastTransaction(eventType string, txn purchase.Transaction) {
	h.broadcastEvent(eventType, txn)
}

func (h *Hub) broadcastEvent(eventType string, data any) {
	msg, err := json.Marshal(Event{ID: uuid.NewString(), Type: eventType, OccurredAt: time.Now().UTC(), Data: data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // slow client; drop rather than block the broadcaster
		}
	}
}

func (h *Hub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}
