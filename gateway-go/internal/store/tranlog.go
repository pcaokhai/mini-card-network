package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// TranLogRow is one tran_log row (docs/05-data-model.md). RRN and MaskedPAN are never the real
// PAN; the caller resolves cardToken->PAN only long enough to build DE 2 and never persists it.
type TranLogRow struct {
	RRN          string
	Type         string
	Status       string
	Amount       int64
	Currency     string
	MaskedPAN    string
	TerminalID   string
	MerchantID   string
	NetworkSTAN  string
	ResponseCode string
	AuthCode     string
}

// TranLogRepository persists tran_log and tran_state_history.
type TranLogRepository struct{ pool *Pool }

// NewTranLogRepository builds a TranLogRepository backed by pool.
func NewTranLogRepository(pool *Pool) *TranLogRepository { return &TranLogRepository{pool: pool} }

// Insert records a new tran_log row and returns its id.
func (r *TranLogRepository) Insert(ctx context.Context, row TranLogRow) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO tran_log (business_date, client_request_id, tran_type, tid, mid, network_stan, rrn, masked_pan, amount, currency, state)
		 VALUES (CURRENT_DATE, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id`,
		row.RRN, row.Type, row.TerminalID, row.MerchantID, row.NetworkSTAN, row.RRN, row.MaskedPAN, row.Amount, row.Currency, row.Status,
	).Scan(&id)
	return id, err
}

// UpdateStatus sets the current state of the tran_log row identified by id.
func (r *TranLogRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tran_log SET state = $2, responded_at = now() WHERE id = $1`, id, status)
	return err
}

// RecordStateTransition appends a tran_state_history row for id's move from fromStatus to toStatus.
func (r *TranLogRepository) RecordStateTransition(ctx context.Context, id int64, fromStatus, toStatus string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tran_state_history (tran_id, from_state, to_state) VALUES ($1, $2, $3)`, id, fromStatus, toStatus)
	return err
}

// Get reads the tran_log row for the given RRN.
func (r *TranLogRepository) Get(ctx context.Context, rrn string) (TranLogRow, error) {
	var row TranLogRow
	err := r.pool.QueryRow(ctx,
		`SELECT rrn, tran_type, state, amount, currency, masked_pan, tid, mid FROM tran_log WHERE rrn = $1`, rrn,
	).Scan(&row.RRN, &row.Type, &row.Status, &row.Amount, &row.Currency, &row.MaskedPAN, &row.TerminalID, &row.MerchantID)
	return row, err
}

// StoredResponse is a previously stored idempotent response.
type StoredResponse struct {
	Status int
	Body   []byte
}

// IdempotencyRepository persists idempotency_record for replay of state-changing REST calls.
type IdempotencyRepository struct{ pool *Pool }

// NewIdempotencyRepository builds an IdempotencyRepository backed by pool.
func NewIdempotencyRepository(pool *Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

// Find returns the stored response for (key, route), or nil if no such key was ever stored.
func (r *IdempotencyRepository) Find(ctx context.Context, key, route string) (*StoredResponse, error) {
	var resp StoredResponse
	err := r.pool.QueryRow(ctx,
		`SELECT status, body FROM idempotency_record WHERE key = $1 AND route = $2`, key, route,
	).Scan(&resp.Status, &resp.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// Store records the response returned for (key, route) so a replay can return it unchanged.
func (r *IdempotencyRepository) Store(ctx context.Context, key, route, requestHash string, status int, body []byte) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO idempotency_record (key, route, request_hash, status, body, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		key, route, requestHash, status, body, time.Now().UTC())
	return err
}
