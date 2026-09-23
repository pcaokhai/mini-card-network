package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a lookup by RRN finds no row.
var ErrNotFound = errors.New("not found")

// TranLogRow is one tran_log row (docs/05-data-model.md). RRN and MaskedPAN are never the real
// PAN; the caller resolves cardToken->PAN only long enough to build DE 2 and never persists it.
type TranLogRow struct {
	ID               int64
	RRN              string
	Type             string
	Status           string
	Amount           int64
	Currency         string
	MaskedPAN        string
	TerminalID       string
	MerchantID       string
	MerchantName     string
	NetworkSTAN      string
	ResponseCode     string
	AuthCode         string
	CreatedAt        time.Time
	LateResponseCode string
	LateResponseAt   *time.Time
}

// TransactionFilter narrows TranLogRepository.List. Nil pointer fields mean "no filter".
type TransactionFilter struct {
	Status *string
	RC     *string
	Last4  *string
	From   *time.Time
	To     *time.Time
	Cursor string
	Limit  int
}

const defaultTransactionsLimit = 50

// StateTransition is one tran_state_history row.
type StateTransition struct {
	FromStatus string
	ToStatus   string
	At         time.Time
}

// TranLogRepository persists tran_log and tran_state_history.
type TranLogRepository struct{ pool *Pool }

// NewTranLogRepository builds a TranLogRepository backed by pool.
func NewTranLogRepository(pool *Pool) *TranLogRepository { return &TranLogRepository{pool: pool} }

// Insert records a new tran_log row and returns its id.
func (r *TranLogRepository) Insert(ctx context.Context, row TranLogRow) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO tran_log (business_date, client_request_id, tran_type, tid, mid, network_stan, rrn, masked_pan, amount, currency, state, response_code, auth_code)
		 VALUES (CURRENT_DATE, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''))
		 RETURNING id`,
		row.RRN, row.Type, row.TerminalID, row.MerchantID, row.NetworkSTAN, row.RRN, row.MaskedPAN, row.Amount, row.Currency, row.Status, row.ResponseCode, row.AuthCode,
	).Scan(&id)
	return id, err
}

// UpdateStatus sets the current state, response code, and auth code of the tran_log row
// identified by id. responseCode/authCode may be empty when not yet known (e.g. the CREATED->SENT
// transition, before the issuer has responded).
func (r *TranLogRepository) UpdateStatus(ctx context.Context, id int64, status, responseCode, authCode string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tran_log SET state = $2, response_code = NULLIF($3, ''), auth_code = NULLIF($4, ''), responded_at = now() WHERE id = $1`,
		id, status, responseCode, authCode)
	return err
}

// UpdateLateResponse records a response that arrived for rrn after its transaction already moved
// on to a final status. It never touches state - the transaction is already final (MCN-403-AC1).
func (r *TranLogRepository) UpdateLateResponse(ctx context.Context, rrn, responseCode string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tran_log SET late_response_code = $2, late_response_at = now() WHERE rrn = $1`,
		rrn, responseCode)
	return err
}

// RecordStateTransition appends a tran_state_history row for id's move from fromStatus to toStatus.
func (r *TranLogRepository) RecordStateTransition(ctx context.Context, id int64, fromStatus, toStatus string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tran_state_history (tran_id, from_state, to_state) VALUES ($1, $2, $3)`, id, fromStatus, toStatus)
	return err
}

// ListStateHistory returns id's tran_state_history rows in transition order (oldest first).
func (r *TranLogRepository) ListStateHistory(ctx context.Context, id int64) ([]StateTransition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT from_state, to_state, created_at FROM tran_state_history WHERE tran_id = $1 ORDER BY created_at ASC, id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []StateTransition
	for rows.Next() {
		var st StateTransition
		var from *string
		if err := rows.Scan(&from, &st.ToStatus, &st.At); err != nil {
			return nil, err
		}
		if from != nil {
			st.FromStatus = *from
		}
		history = append(history, st)
	}
	return history, rows.Err()
}

const tranLogSelectColumns = `t.id, t.rrn, t.tran_type, t.state, t.amount, t.currency, t.masked_pan, t.tid, t.mid, m.name, coalesce(t.response_code, ''), coalesce(t.auth_code, ''), t.created_at, coalesce(t.late_response_code, ''), t.late_response_at`

// Get reads the tran_log row for the given RRN.
func (r *TranLogRepository) Get(ctx context.Context, rrn string) (TranLogRow, error) {
	var row TranLogRow
	err := r.pool.QueryRow(ctx,
		`SELECT `+tranLogSelectColumns+` FROM tran_log t JOIN merchant m ON m.mid = t.mid WHERE t.rrn = $1`, rrn,
	).Scan(&row.ID, &row.RRN, &row.Type, &row.Status, &row.Amount, &row.Currency, &row.MaskedPAN, &row.TerminalID, &row.MerchantID, &row.MerchantName, &row.ResponseCode, &row.AuthCode, &row.CreatedAt, &row.LateResponseCode, &row.LateResponseAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TranLogRow{}, ErrNotFound
	}
	row.RRN = strings.TrimSpace(row.RRN)
	return row, err
}

// List returns a page of tran_log rows matching filter, newest first, plus an opaque cursor for
// the next page (empty when there is no next page). The cursor is a keyset on (created_at, id) —
// not an offset — so it stays stable when rows are concurrently inserted.
func (r *TranLogRepository) List(ctx context.Context, filter TransactionFilter) ([]TranLogRow, string, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultTransactionsLimit
	}

	query, args, err := buildListQuery(filter, limit)
	if err != nil {
		return nil, "", err
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	page, err := scanTranLogRows(rows)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(page) > limit {
		last := page[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		page = page[:limit]
	}
	return page, nextCursor, nil
}

// buildListQuery builds the filtered, cursor-paginated SELECT for List. It fetches limit+1 rows
// so List can tell whether a next page exists.
func buildListQuery(filter TransactionFilter, limit int) (string, []any, error) {
	query := `SELECT ` + tranLogSelectColumns + ` FROM tran_log t JOIN merchant m ON m.mid = t.mid WHERE 1=1`
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.Status != nil {
		query += ` AND t.state = ` + arg(*filter.Status)
	}
	if filter.RC != nil {
		query += ` AND t.response_code = ` + arg(*filter.RC)
	}
	if filter.Last4 != nil {
		query += ` AND right(t.masked_pan, 4) = ` + arg(*filter.Last4)
	}
	if filter.From != nil {
		query += ` AND t.created_at >= ` + arg(*filter.From)
	}
	if filter.To != nil {
		query += ` AND t.created_at <= ` + arg(*filter.To)
	}
	if filter.Cursor != "" {
		cursorAt, cursorID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return "", nil, fmt.Errorf("decode cursor: %w", err)
		}
		query += ` AND (t.created_at, t.id) < (` + arg(cursorAt) + `, ` + arg(cursorID) + `)`
	}
	query += ` ORDER BY t.created_at DESC, t.id DESC LIMIT ` + arg(limit+1)
	return query, args, nil
}

func scanTranLogRows(rows pgx.Rows) ([]TranLogRow, error) {
	var page []TranLogRow
	for rows.Next() {
		var row TranLogRow
		if err := rows.Scan(&row.ID, &row.RRN, &row.Type, &row.Status, &row.Amount, &row.Currency, &row.MaskedPAN, &row.TerminalID, &row.MerchantID, &row.MerchantName, &row.ResponseCode, &row.AuthCode, &row.CreatedAt, &row.LateResponseCode, &row.LateResponseAt); err != nil {
			return nil, err
		}
		row.RRN = strings.TrimSpace(row.RRN)
		page = append(page, row)
	}
	return page, rows.Err()
}

// encodeCursor builds an opaque base64 keyset cursor from (created_at, id).
func encodeCursor(at time.Time, id int64) string {
	raw := fmt.Sprintf("%d|%d", at.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor reverses encodeCursor.
func decodeCursor(cursor string) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("malformed cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Unix(0, nanos).UTC(), id, nil
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
