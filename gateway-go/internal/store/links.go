package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

// ErrInvalidCursor means a cursor (network events, transactions) is not one this store issued.
var ErrInvalidCursor = errors.New("invalid cursor")

// Link is one row of link_state. JSON shape matches contracts/openapi.yaml's Link schema
// (linkId/from/to/lastEchoOk are derived, not stored, since v1 has exactly one link).
type Link struct {
	Endpoint          string
	Status            string
	LastEchoAt        *time.Time
	LastEchoLatencyMs *int
	P99LatencyMs      *int
	InFlight          int
}

const gatewayEndpoint = "gateway"

// MarshalJSON emits the contracts/openapi.yaml Link shape.
func (l Link) MarshalJSON() ([]byte, error) {
	var lastEchoOk *bool
	if l.LastEchoAt != nil {
		ok := true
		lastEchoOk = &ok
	}
	return json.Marshal(struct {
		LinkID       string     `json:"linkId"`
		From         string     `json:"from"`
		To           string     `json:"to"`
		Status       string     `json:"status"`
		LastEchoAt   *time.Time `json:"lastEchoAt"`
		LastEchoOk   *bool      `json:"lastEchoOk"`
		P99LatencyMs *int       `json:"p99LatencyMs"`
		InFlight     int        `json:"inFlight"`
	}{l.Endpoint, gatewayEndpoint, l.Endpoint, l.Status, l.LastEchoAt, lastEchoOk, l.P99LatencyMs, l.InFlight})
}

// NetworkEvent is one row of network_event. JSON shape matches contracts/openapi.yaml's
// NetworkEvent schema (id is a string on the wire).
type NetworkEvent struct {
	ID            int64
	OccurredAt    time.Time
	Code          string // contracts NetworkEventCode; empty only on rows older than migration 00008
	Severity      string
	EasyText      string
	TechnicalText string
}

// MarshalJSON emits the contracts/openapi.yaml NetworkEvent shape.
func (e NetworkEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID            string    `json:"id"`
		OccurredAt    time.Time `json:"occurredAt"`
		Severity      string    `json:"severity"`
		Code          string    `json:"code,omitempty"`
		EasyText      string    `json:"easyText"`
		TechnicalText string    `json:"technicalText"`
	}{strconv.FormatInt(e.ID, 10), e.OccurredAt, e.Severity, e.Code, e.EasyText, e.TechnicalText})
}

// LinkRepository persists link_state and network_event.
type LinkRepository struct{ pool *Pool }

// NewLinkRepository builds a LinkRepository backed by pool.
func NewLinkRepository(pool *Pool) *LinkRepository { return &LinkRepository{pool: pool} }

// SetStatus updates the status of the named link.
func (r *LinkRepository) SetStatus(ctx context.Context, endpoint, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE link_state SET status = $2, updated_at = now() WHERE endpoint = $1`, endpoint, status)
	return err
}

// RecordEcho records a successful echo's latency for the named link.
func (r *LinkRepository) RecordEcho(ctx context.Context, endpoint string, latencyMs int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE link_state SET last_echo_at = now(), last_echo_latency_ms = $2, updated_at = now() WHERE endpoint = $1`,
		endpoint, latencyMs)
	return err
}

// Get reads the current state of the named link.
func (r *LinkRepository) Get(ctx context.Context, endpoint string) (Link, error) {
	var l Link
	err := r.pool.QueryRow(ctx,
		`SELECT endpoint, status, last_echo_at, last_echo_latency_ms, p99_latency_ms, in_flight FROM link_state WHERE endpoint = $1`,
		endpoint).Scan(&l.Endpoint, &l.Status, &l.LastEchoAt, &l.LastEchoLatencyMs, &l.P99LatencyMs, &l.InFlight)
	return l, err
}

// RecordEvent inserts a network_event row without a code.
//
// Deprecated: use AppendEvent, which records the code and returns the stored row.
func (r *LinkRepository) RecordEvent(ctx context.Context, severity, easyText, technicalText string) error {
	_, err := r.AppendEvent(ctx, NetworkEvent{Severity: severity, EasyText: easyText, TechnicalText: technicalText})
	return err
}

// AppendEvent inserts a network_event row and returns it as stored (id, occurredAt), so a
// broadcast carries the same resource GET /v1/network/events serves (NET-G4).
func (r *LinkRepository) AppendEvent(ctx context.Context, e NetworkEvent) (NetworkEvent, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO network_event (code, severity, easy_text, technical_text) VALUES (NULLIF($1, ''), $2, $3, $4)
		 RETURNING id, occurred_at`,
		e.Code, e.Severity, e.EasyText, e.TechnicalText).Scan(&e.ID, &e.OccurredAt)
	return e, err
}

// ListEvents returns up to limit network_event rows, newest first, starting after cursor ("" for
// the first page). The cursor is the last row's id; nextCursor is "" on the last page (NET-G12).
func (r *LinkRepository) ListEvents(ctx context.Context, limit int, cursor string) ([]NetworkEvent, string, error) {
	var before int64
	if cursor != "" {
		n, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || n <= 0 {
			return nil, "", ErrInvalidCursor
		}
		before = n
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, occurred_at, coalesce(code, ''), severity, easy_text, technical_text FROM network_event
		 WHERE $2 = 0 OR id < $2 ORDER BY id DESC LIMIT $1`, limit+1, before)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var events []NetworkEvent
	for rows.Next() {
		var e NetworkEvent
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Code, &e.Severity, &e.EasyText, &e.TechnicalText); err != nil {
			return nil, "", err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(events) <= limit {
		return events, "", nil
	}
	events = events[:limit]
	return events, strconv.FormatInt(events[limit-1].ID, 10), nil
}
