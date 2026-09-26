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

// ErrNotReversible means the transaction is not in a state a reversal can start from: it holds no
// money (a decline), or a reversal is already queued or done.
var ErrNotReversible = errors.New("transaction cannot be reversed from its current state")

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
	// What a reversal advice must repeat from the original request (docs/03 §7.3).
	MTI            string // the request's MTI (0100, 0200, 0220); DE 90 names it
	ProcessingCode string
	POSEntryMode   string
	SentAt         *time.Time // the moment DE 7 was built from
	RespondedAt    *time.Time // set by UpdateStatus; the issuer's answer once the status is final
	CardToken      string     // simulator token, never the PAN
	// Detail the Transaction resource reports (JRN-G3, POS-G8).
	ApprovedAmount *int64 // partial approval (RC 10): DE 4 of the response
	Balance        *Money // balance inquiry: DE 54 of the response
	OriginalRRN    string // completion: the pre-authorization it completes
	TraceID        string // W3C trace id of the request that created the row
	// ReversalReasonCode is DE 39 of the 0420 queued for this row, empty when none was.
	ReversalReasonCode string
	// CompletedBy is the RRN of the completion that consumed this pre-authorization.
	CompletedBy string
}

// Money is an amount in integer minor units and its ISO 4217 numeric currency.
type Money struct {
	Amount   int64
	Currency string
}

// TransactionFilter narrows TranLogRepository.List. Nil pointer fields mean "no filter".
type TransactionFilter struct {
	Status *string
	RC     *string
	Last4  *string
	From   *time.Time
	To     *time.Time
	// ReversalReason is one of the Reversal* values.
	ReversalReason *string
	Cursor         string
	Limit          int
}

const defaultTransactionsLimit = 50

// Reversal reasons (contracts/openapi.yaml ReversalReason), from DE 39 of the 0420 (docs/03 §7.3).
const (
	ReversalCustomerCancellation = "CUSTOMER_CANCELLATION"
	ReversalTimeout              = "TIMEOUT"
	ReversalMACFailure           = "MAC_FAILURE"
	ReversalSendFailure          = "SEND_FAILURE"
)

var reversalReasonByCode = map[string]string{"17": ReversalCustomerCancellation, "68": ReversalTimeout, "06": ReversalMACFailure}

// ReversalReasonOf names a 0420's DE 39 reason code; empty when no reversal was queued. A code
// other than 17, 68 and 06 is a SEND_FAILURE.
func ReversalReasonOf(code string) string {
	if code == "" {
		return ""
	}
	if reason, ok := reversalReasonByCode[code]; ok {
		return reason
	}
	return ReversalSendFailure
}

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

// ErrNotCompletable means a completion names a transaction that is not an approved
// pre-authorization, or one another completion already consumed.
var ErrNotCompletable = errors.New("only an approved, uncompleted pre-authorization can be completed")

// Insert records a new tran_log row and returns its id.
func (r *TranLogRepository) Insert(ctx context.Context, row TranLogRow) (int64, error) {
	return insertTranLog(ctx, r.pool, row)
}

// InsertCompletion records completion and claims the pre-authorization preAuthRRN for it in one
// DB transaction, so a pre-auth is completed at most once even under concurrent requests. A
// pre-auth that isn't APPROVED, or was already completed, is ErrNotCompletable and nothing is
// recorded.
func (r *TranLogRepository) InsertCompletion(ctx context.Context, completion TranLogRow, preAuthRRN string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	claimed, err := tx.Exec(ctx,
		`UPDATE tran_log SET completed_by = $2
		 WHERE rrn = $1 AND tran_type = 'PREAUTH' AND state = 'APPROVED' AND completed_by IS NULL`,
		preAuthRRN, completion.RRN)
	if err != nil {
		return 0, err
	}
	if claimed.RowsAffected() == 0 {
		return 0, fmt.Errorf("complete %s: %w", preAuthRRN, ErrNotCompletable)
	}
	id, err := insertTranLog(ctx, tx, completion)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// rowQuerier is what insertTranLog needs: a pool or a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func insertTranLog(ctx context.Context, q rowQuerier, row TranLogRow) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO tran_log (business_date, client_request_id, tran_type, tid, mid, network_stan, rrn, masked_pan, amount, currency, state, response_code, auth_code,
		                       processing_code, pos_entry_mode, sent_at, card_token, mti, original_rrn, trace_id)
		 VALUES (CURRENT_DATE, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''),
		         NULLIF($13, ''), NULLIF($14, ''), $15, NULLIF($16, ''), NULLIF($17, ''), NULLIF($18, ''), NULLIF($19, ''))
		 RETURNING id`,
		row.RRN, row.Type, row.TerminalID, row.MerchantID, row.NetworkSTAN, row.RRN, row.MaskedPAN, row.Amount, row.Currency, row.Status, row.ResponseCode, row.AuthCode,
		row.ProcessingCode, row.POSEntryMode, row.SentAt, row.CardToken, row.MTI, row.OriginalRRN, row.TraceID,
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

// UpdateAmounts records what an issuer response reported beyond its RC: a partial approval's
// approved amount and a balance inquiry's balance (nil when the response carried none).
func (r *TranLogRepository) UpdateAmounts(ctx context.Context, id int64, approvedAmount *int64, balance *Money) error {
	var balanceAmount *int64
	var balanceCurrency *string
	if balance != nil {
		balanceAmount, balanceCurrency = &balance.Amount, &balance.Currency
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE tran_log SET approved_amount = $2, balance_amount = $3, balance_currency = $4 WHERE id = $1`,
		id, approvedAmount, balanceAmount, balanceCurrency)
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

const tranLogSelectColumns = `t.id, t.rrn, t.tran_type, t.state, t.amount, t.currency, t.masked_pan, t.tid, t.mid, m.name, coalesce(t.response_code, ''), coalesce(t.auth_code, ''), t.created_at, coalesce(t.late_response_code, ''), t.late_response_at, coalesce(t.network_stan, ''), coalesce(t.processing_code, ''), coalesce(t.pos_entry_mode, ''), t.sent_at, coalesce(t.card_token, ''), t.responded_at, coalesce(t.mti, ''),
	t.approved_amount, t.balance_amount, coalesce(t.balance_currency, ''), coalesce(t.original_rrn, ''), coalesce(t.trace_id, ''), coalesce(t.reversal_reason, ''), coalesce(t.completed_by, '')`

// Get reads the tran_log row for the given RRN.
func (r *TranLogRepository) Get(ctx context.Context, rrn string) (TranLogRow, error) {
	row, err := scanTranLogRow(r.pool.QueryRow(ctx,
		`SELECT `+tranLogSelectColumns+` FROM tran_log t JOIN merchant m ON m.mid = t.mid WHERE t.rrn = $1`, rrn))
	if errors.Is(err, pgx.ErrNoRows) {
		return TranLogRow{}, ErrNotFound
	}
	return row, err
}

// scanTranLogRow reads one row selected with tranLogSelectColumns.
func scanTranLogRow(scanner pgx.Row) (TranLogRow, error) {
	var row TranLogRow
	var balanceAmount *int64
	var balanceCurrency string
	err := scanner.Scan(&row.ID, &row.RRN, &row.Type, &row.Status, &row.Amount, &row.Currency, &row.MaskedPAN, &row.TerminalID, &row.MerchantID, &row.MerchantName, &row.ResponseCode, &row.AuthCode, &row.CreatedAt, &row.LateResponseCode, &row.LateResponseAt,
		&row.NetworkSTAN, &row.ProcessingCode, &row.POSEntryMode, &row.SentAt, &row.CardToken, &row.RespondedAt, &row.MTI,
		&row.ApprovedAmount, &balanceAmount, &balanceCurrency, &row.OriginalRRN, &row.TraceID, &row.ReversalReasonCode, &row.CompletedBy)
	if err != nil {
		return TranLogRow{}, err
	}
	if balanceAmount != nil {
		row.Balance = &Money{Amount: *balanceAmount, Currency: balanceCurrency}
	}
	row.RRN = strings.TrimSpace(row.RRN)
	row.OriginalRRN = strings.TrimSpace(row.OriginalRRN)
	row.CompletedBy = strings.TrimSpace(row.CompletedBy)
	return row, nil
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
	if filter.ReversalReason != nil {
		query += reversalReasonClause(*filter.ReversalReason, arg)
	}
	if filter.Cursor != "" {
		cursorAt, cursorID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
		}
		query += ` AND (t.created_at, t.id) < (` + arg(cursorAt) + `, ` + arg(cursorID) + `)`
	}
	query += ` ORDER BY t.created_at DESC, t.id DESC LIMIT ` + arg(limit+1)
	return query, args, nil
}

// reversalReasonClause matches the codes reason names; SEND_FAILURE is every other code.
func reversalReasonClause(reason string, arg func(any) string) string {
	for code, name := range reversalReasonByCode {
		if name == reason {
			return ` AND t.reversal_reason = ` + arg(code)
		}
	}
	return ` AND t.reversal_reason NOT IN ('17', '68', '06')`
}

func scanTranLogRows(rows pgx.Rows) ([]TranLogRow, error) {
	var page []TranLogRow
	for rows.Next() {
		row, err := scanTranLogRow(rows)
		if err != nil {
			return nil, err
		}
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

// InProgressError is ErrIdempotencyInProgress with what the first request recorded: the RRN it
// sent (empty until it sent) and when it reserved the key.
type InProgressError struct {
	RRN        string
	ReservedAt time.Time
}

func (e *InProgressError) Error() string { return ErrIdempotencyInProgress.Error() }

// Is makes an InProgressError match ErrIdempotencyInProgress.
func (e *InProgressError) Is(target error) bool { return target == ErrIdempotencyInProgress }

// Idempotency errors (docs/04 §2).
var (
	ErrIdempotencyKeyMismatch = errors.New("idempotency key reused with a different request")
	ErrIdempotencyInProgress  = errors.New("a request with this idempotency key is still in progress")
)

// idempotencyTTL is how long a key replays its response (docs/04 §2).
const idempotencyTTL = "24 hours"

// pendingStatus marks a reserved key whose request hasn't stored its response yet.
const pendingStatus = 0

// IdempotencyRepository persists idempotency_record for replay of state-changing REST calls.
type IdempotencyRepository struct{ pool *Pool }

// NewIdempotencyRepository builds an IdempotencyRepository backed by pool.
func NewIdempotencyRepository(pool *Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

// Reserve claims (key, route) for the request hashed requestHash, atomically: exactly one of
// several concurrent callers gets (nil, token, nil) and may go on to send, fencing everything it
// does with the key by token. A later caller gets the stored response to replay, an
// *InProgressError while the first hasn't finished, or ErrIdempotencyKeyMismatch for a different
// request. A key older than 24 h is claimed afresh.
func (r *IdempotencyRepository) Reserve(ctx context.Context, key, route, requestHash string) (*StoredResponse, string, error) {
	var token string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO idempotency_record (key, route, request_hash, status, body, created_at, reservation_token)
		 VALUES ($1, $2, $3, $4, 'null', now(), gen_random_uuid())
		 ON CONFLICT (key, route) DO UPDATE
		   SET request_hash = EXCLUDED.request_hash, status = EXCLUDED.status, body = EXCLUDED.body, created_at = EXCLUDED.created_at,
		       rrn = NULL, reservation_token = EXCLUDED.reservation_token
		   WHERE idempotency_record.created_at < now() - interval '`+idempotencyTTL+`'
		 RETURNING reservation_token::text`,
		key, route, requestHash, pendingStatus).Scan(&token)
	if err == nil {
		return nil, token, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}
	var storedHash string
	var resp StoredResponse
	var pending InProgressError
	err = r.pool.QueryRow(ctx,
		`SELECT request_hash, status, body, coalesce(rrn, ''), created_at FROM idempotency_record WHERE key = $1 AND route = $2`, key, route,
	).Scan(&storedHash, &resp.Status, &resp.Body, &pending.RRN, &pending.ReservedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, "", ErrIdempotencyInProgress // released between the two statements
	case err != nil:
		return nil, "", err
	case storedHash != requestHash:
		return nil, "", ErrIdempotencyKeyMismatch
	case resp.Status == pendingStatus:
		pending.RRN = strings.TrimSpace(pending.RRN)
		return nil, "", &pending
	}
	return &resp, "", nil
}

// Store records the response returned for (key, route) so a replay can return it unchanged.
func (r *IdempotencyRepository) Store(ctx context.Context, key, route, requestHash string, status int, body []byte) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO idempotency_record (key, route, request_hash, status, body, created_at) VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (key, route) DO UPDATE SET request_hash = EXCLUDED.request_hash, status = EXCLUDED.status, body = EXCLUDED.body`,
		key, route, requestHash, status, body)
	return err
}

// ErrReservationLost means a reservation's token no longer holds its key: another request
// reclaimed it while this one was slow. The holder must abort without sending or releasing.
var ErrReservationLost = errors.New("idempotency reservation was reclaimed by another request")

// AttachRRN records the RRN a reserved key's request is about to send, before it is sent. It is
// ErrReservationLost unless token still holds the key, so a holder that lost its key never sends.
func (r *IdempotencyRepository) AttachRRN(ctx context.Context, key, route, token, rrn string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE idempotency_record SET rrn = $3
		 WHERE key = $1 AND route = $2 AND status = $4 AND reservation_token::text = $5`, key, route, rrn, pendingStatus, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrReservationLost
	}
	return nil
}

// Release frees a reservation whose request failed before anything was sent, so the key can be
// retried. Only token's own reservation is freed: a stored response, or a reservation another
// request reclaimed, is left alone.
func (r *IdempotencyRepository) Release(ctx context.Context, key, route, token string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM idempotency_record WHERE key = $1 AND route = $2 AND status = $3 AND reservation_token::text = $4`,
		key, route, pendingStatus, token)
	return err
}

// LastSTANInRRNPrefix returns the highest STAN used in an RRN starting with prefix ("Y DDD hh",
// docs/03 §5), or 0 when there is none: where a restarted gateway resumes its STAN count.
func (r *TranLogRepository) LastSTANInRRNPrefix(ctx context.Context, prefix string) (int64, error) {
	var last int64
	err := r.pool.QueryRow(ctx,
		`SELECT coalesce(max(substring(rrn FROM 7 FOR 6)::int), 0) FROM tran_log WHERE rrn LIKE $1 || '%' AND rrn ~ '^[0-9]{12}$'`,
		prefix).Scan(&last)
	return last, err
}

// backdateStatements shift one transaction's journey timestamps ($1 = tran id, $2 = seconds).
var backdateStatements = []string{
	`UPDATE tran_log SET created_at = created_at + make_interval(secs => $2),
	   sent_at = sent_at + make_interval(secs => $2),
	   responded_at = responded_at + make_interval(secs => $2),
	   late_response_at = late_response_at + make_interval(secs => $2)
	 WHERE id = $1`,
	`UPDATE tran_state_history SET created_at = created_at + make_interval(secs => $2) WHERE tran_id = $1`,
	`UPDATE saf_queue SET created_at = created_at + make_interval(secs => $2),
	   next_retry_at = next_retry_at + make_interval(secs => $2),
	   acked_at = acked_at + make_interval(secs => $2)
	 WHERE tran_id = $1`,
}

// Backdate moves rrn's tran_log.created_at to at and shifts its sent and response times, its
// tran_state_history and its saf_queue rows by the same amount, so its latencies are unchanged.
// Seed-only: it rewrites the acquirer's record of when a transaction happened, which the issuer's
// ledger does not follow (docs/plans/MCN-002-acquirer-seed.md ruling 2).
func (r *TranLogRepository) Backdate(ctx context.Context, rrn string, at time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var id int64
	var createdAt time.Time
	err = tx.QueryRow(ctx, `SELECT id, created_at FROM tran_log WHERE rrn = $1 FOR UPDATE`, rrn).Scan(&id, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	secs := at.Sub(createdAt).Seconds()
	// Every timestamp the journey reads moves by the same shift, so the story keeps its spacing.
	for _, stmt := range backdateStatements {
		if _, err := tx.Exec(ctx, stmt, id, secs); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReleaseCompletion undoes completionRRN's claim on preAuthRRN, so the hold can be completed
// again: the completion never left the gateway, or the issuer declined it. A claim held by
// another completion is left alone.
func (r *TranLogRepository) ReleaseCompletion(ctx context.Context, preAuthRRN, completionRRN string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tran_log SET completed_by = NULL WHERE rrn = $1 AND completed_by = $2`, preAuthRRN, completionRRN)
	return err
}

// Reclaim takes over a reservation of (key, route) for the same request that has been pending
// longer than staleAfter without recording an RRN, returning the new token ("" when not
// reclaimed). AttachRRN always runs before the send, so such a key sent nothing yet; its holder,
// if only slow, now holds a stale token and can neither send nor release. Reclaiming refreshes
// the reservation's age, so of several concurrent retries exactly one wins.
func (r *IdempotencyRepository) Reclaim(ctx context.Context, key, route, requestHash string, staleAfter time.Duration) (string, error) {
	var token string
	err := r.pool.QueryRow(ctx,
		`UPDATE idempotency_record SET created_at = now(), reservation_token = gen_random_uuid()
		 WHERE key = $1 AND route = $2 AND request_hash = $3 AND status = $4 AND rrn IS NULL
		   AND created_at < now() - make_interval(secs => $5)
		 RETURNING reservation_token::text`,
		key, route, requestHash, pendingStatus, staleAfter.Seconds()).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return token, err
}

// FindOrphans returns up to limit rows sent before sentBefore whose outcome was never followed
// up (POS-G16): still SENT - the gateway stopped between the send and recording the answer - or
// TIMED_OUT with nothing queued in saf_queue - it stopped, or failed, before queueing the reversal
// or the advice repeat - or DECLINED RC 96 (a bad incoming MAC) with nothing queued, whose
// reason-06 reversal was never queued. A balance inquiry owes nothing, so it is never an orphan
// once it has left SENT, and a completion's bad MAC leaves it TIMED_OUT, not DECLINED.
func (r *TranLogRepository) FindOrphans(ctx context.Context, sentBefore time.Time, limit int) ([]TranLogRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+tranLogSelectColumns+` FROM tran_log t JOIN merchant m ON m.mid = t.mid
		WHERE t.sent_at < $1
		  AND (t.state = 'SENT'
		       OR (t.state = 'TIMED_OUT' AND t.tran_type <> 'BALANCE'
		           AND NOT EXISTS (SELECT 1 FROM saf_queue s WHERE s.tran_id = t.id))
		       OR (t.state = 'DECLINED' AND t.response_code = '96' AND t.tran_type NOT IN ('BALANCE', 'COMPLETION')
		           AND NOT EXISTS (SELECT 1 FROM saf_queue s WHERE s.tran_id = t.id)))
		ORDER BY t.id LIMIT $2`, sentBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTranLogRows(rows)
}

// MarkTimedOut moves row id from SENT to TIMED_OUT, recording the transition, and reports whether
// it moved; a row that already left SENT is left alone.
func (r *TranLogRepository) MarkTimedOut(ctx context.Context, id int64) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	moved, err := tx.Exec(ctx, `UPDATE tran_log SET state = 'TIMED_OUT' WHERE id = $1 AND state = 'SENT'`, id)
	if err != nil || moved.RowsAffected() == 0 {
		return false, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tran_state_history (tran_id, from_state, to_state) VALUES ($1, 'SENT', 'TIMED_OUT')`, id); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// ErrStateMoved means a guarded update found the row no longer in the state it expected.
var ErrStateMoved = errors.New("transaction state moved on")

// UpdateStatusFrom is UpdateStatus only while row id is still in from: a request's late outcome
// never overwrites a row the orphan sweeper already moved (#123 review N1).
func (r *TranLogRepository) UpdateStatusFrom(ctx context.Context, id int64, from, status, responseCode, authCode string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tran_log SET state = $2, response_code = NULLIF($3, ''), auth_code = NULLIF($4, ''), responded_at = now()
		 WHERE id = $1 AND state = $5`,
		id, status, responseCode, authCode, from)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update tran_log %d from %s to %s: %w", id, from, status, ErrStateMoved)
	}
	return nil
}
