package store

import (
	"context"
	"time"
)

// Link is one row of link_state.
type Link struct {
	Endpoint          string
	Status            string
	LastEchoAt        *time.Time
	LastEchoLatencyMs *int
	P99LatencyMs      *int
	InFlight          int
}

// NetworkEvent is one row of network_event.
type NetworkEvent struct {
	ID            int64
	OccurredAt    time.Time
	Severity      string
	EasyText      string
	TechnicalText string
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

// RecordEvent inserts a network_event row.
func (r *LinkRepository) RecordEvent(ctx context.Context, severity, easyText, technicalText string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO network_event (severity, easy_text, technical_text) VALUES ($1, $2, $3)`,
		severity, easyText, technicalText)
	return err
}

// ListEvents returns the most recent network_event rows, newest first.
func (r *LinkRepository) ListEvents(ctx context.Context, limit int) ([]NetworkEvent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, occurred_at, severity, easy_text, technical_text FROM network_event ORDER BY occurred_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []NetworkEvent
	for rows.Next() {
		var e NetworkEvent
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Severity, &e.EasyText, &e.TechnicalText); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
