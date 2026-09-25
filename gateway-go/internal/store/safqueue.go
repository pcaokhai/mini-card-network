package store

import (
	"context"
	"time"
)

// SafRow is one saf_queue row (docs/05-data-model.md, docs/03 §7.3).
type SafRow struct {
	ID          int64
	TranID      int64
	MTI         string
	Payload     []byte
	Status      string
	Attempts    int
	MaxAttempts int
	NextRetryAt time.Time
	LastError   string
}

// SafRepository persists saf_queue.
type SafRepository struct{ pool *Pool }

// NewSafRepository builds a SafRepository backed by pool.
func NewSafRepository(pool *Pool) *SafRepository { return &SafRepository{pool: pool} }

// Enqueue records a new PENDING saf_queue row and returns its id.
func (r *SafRepository) Enqueue(ctx context.Context, tranID int64, mti string, payload []byte) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO saf_queue (tran_id, mti, payload_enc) VALUES ($1, $2, $3) RETURNING id`,
		tranID, mti, payload,
	).Scan(&id)
	return id, err
}

// ClaimDue atomically claims up to limit due rows (PENDING, or IN_FLIGHT past their
// next_retry_at - a worker that died mid-delivery leaves rows recoverable this way, MCN-401-AC4)
// and flips them to IN_FLIGHT in the same statement, so no second worker can double-claim between
// the claim and a later MarkInFlight call.
func (r *SafRepository) ClaimDue(ctx context.Context, limit int) ([]SafRow, error) {
	rows, err := r.pool.Query(ctx,
		`WITH claimed AS (
			SELECT id FROM saf_queue
			WHERE status IN ('PENDING', 'IN_FLIGHT') AND next_retry_at <= now()
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE saf_queue SET status = 'IN_FLIGHT'
		FROM claimed WHERE saf_queue.id = claimed.id
		RETURNING saf_queue.id, saf_queue.tran_id, saf_queue.mti, saf_queue.payload_enc, saf_queue.status,
			saf_queue.attempts, saf_queue.max_attempts, saf_queue.next_retry_at, coalesce(saf_queue.last_error, '')`,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []SafRow
	for rows.Next() {
		var row SafRow
		if err := rows.Scan(&row.ID, &row.TranID, &row.MTI, &row.Payload, &row.Status,
			&row.Attempts, &row.MaxAttempts, &row.NextRetryAt, &row.LastError); err != nil {
			return nil, err
		}
		claimed = append(claimed, row)
	}
	return claimed, rows.Err()
}

// UpdatePayload stores id's re-encoded payload: the worker writes an advice's STAN and DE 7 back
// before its first send so every repeat is the same message.
func (r *SafRepository) UpdatePayload(ctx context.Context, id int64, payload []byte) error {
	_, err := r.pool.Exec(ctx, `UPDATE saf_queue SET payload_enc = $2 WHERE id = $1`, id, payload)
	return err
}

// MarkInFlight records a failed delivery attempt and why it failed, and reschedules id for
// nextRetryAt, keeping it IN_FLIGHT so ClaimDue's stale-recovery path also covers it if the worker
// dies before retrying.
func (r *SafRepository) MarkInFlight(ctx context.Context, id int64, attempts int, nextRetryAt time.Time, lastError string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE saf_queue SET status = 'IN_FLIGHT', attempts = $2, next_retry_at = $3, last_error = $4 WHERE id = $1`,
		id, attempts, nextRetryAt, lastError)
	return err
}

// mtiReversalAdvice is the reversal advice whose ACK completes a reversal (docs/03 §7.3).
const mtiReversalAdvice = "0420"

// MarkAcked marks id delivered (the issuer ACKed with 0430 - advices are never declined,
// docs/03 §7.3).
func (r *SafRepository) MarkAcked(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var tranID int64
	var mti string
	if err := tx.QueryRow(ctx,
		`UPDATE saf_queue SET status = 'ACKED', acked_at = now() WHERE id = $1 RETURNING tran_id, mti`, id,
	).Scan(&tranID, &mti); err != nil {
		return err
	}
	// An acknowledged 0420 means the issuer has reversed the money, so the acquirer's record
	// finishes the reversal in the same transaction; advices (0120/0220) don't change state.
	if mti == mtiReversalAdvice {
		tag, err := tx.Exec(ctx,
			`UPDATE tran_log SET state = 'REVERSED' WHERE id = $1 AND state = 'REVERSAL_PENDING'`, tranID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			if _, err := tx.Exec(ctx,
				`INSERT INTO tran_state_history (tran_id, from_state, to_state) VALUES ($1, 'REVERSAL_PENDING', 'REVERSED')`,
				tranID); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// MarkDead marks id DEAD after max_attempts were exhausted without an ACK (MCN-401-AC3).
func (r *SafRepository) MarkDead(ctx context.Context, id int64, lastError string) error {
	_, err := r.pool.Exec(ctx, `UPDATE saf_queue SET status = 'DEAD', last_error = $2 WHERE id = $1`, id, lastError)
	return err
}

// ListPending returns every PENDING/IN_FLIGHT row plus the count of DEAD rows, backing
// GET /v1/network/saf's depth/deadCount.
func (r *SafRepository) ListPending(ctx context.Context) ([]SafRow, int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tran_id, mti, payload_enc, status, attempts, max_attempts, next_retry_at, coalesce(last_error, '')
		 FROM saf_queue WHERE status IN ('PENDING', 'IN_FLIGHT') ORDER BY id`)
	if err != nil {
		return nil, 0, err
	}
	var pending []SafRow
	for rows.Next() {
		var row SafRow
		if err := rows.Scan(&row.ID, &row.TranID, &row.MTI, &row.Payload, &row.Status,
			&row.Attempts, &row.MaxAttempts, &row.NextRetryAt, &row.LastError); err != nil {
			rows.Close()
			return nil, 0, err
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var deadCount int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM saf_queue WHERE status = 'DEAD'`).Scan(&deadCount); err != nil {
		return nil, 0, err
	}
	return pending, deadCount, nil
}

// SafItemRow is one saf_queue row joined with its transaction's rrn/amount/currency, backing
// GET /v1/network/saf's items (contracts/openapi.yaml's SafItem).
type SafItemRow struct {
	ID          int64
	MTI         string
	RRN         string
	AmountMinor int64
	Currency    string
	Attempts    int
	Status      string
	NextRetryAt time.Time
	LastError   string
}

// ListItems returns every non-ACKED saf_queue row (PENDING, IN_FLIGHT, DEAD) with its
// transaction's rrn/amount, plus the DEAD count, backing GET /v1/network/saf.
func (r *SafRepository) ListItems(ctx context.Context) ([]SafItemRow, int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.id, s.mti, t.rrn, t.amount, t.currency, s.attempts, s.status, s.next_retry_at, coalesce(s.last_error, '')
		 FROM saf_queue s JOIN tran_log t ON t.id = s.tran_id
		 WHERE s.status IN ('PENDING', 'IN_FLIGHT', 'DEAD') ORDER BY s.id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []SafItemRow
	deadCount := 0
	for rows.Next() {
		var row SafItemRow
		if err := rows.Scan(&row.ID, &row.MTI, &row.RRN, &row.AmountMinor, &row.Currency,
			&row.Attempts, &row.Status, &row.NextRetryAt, &row.LastError); err != nil {
			return nil, 0, err
		}
		if row.Status == "DEAD" {
			deadCount++
		}
		items = append(items, row)
	}
	return items, deadCount, rows.Err()
}
