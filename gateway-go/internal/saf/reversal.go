package saf

import (
	"context"
	"fmt"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	statusReversalPending = "REVERSAL_PENDING"

	// mtiPurchase is DE 90's original MTI for a row that doesn't record its own (docs/03 §7.3).
	mtiPurchase = "0200"

	// forwardingInstitutionID is right-justified, zero-filled per docs/03 §7.3's DE 90 layout;
	// v1 has no separate forwarding institution, so it is always zeros.
	forwardingInstitutionID = "00000000000"
)

// ReversalQueuer atomically moves a transaction to REVERSAL_PENDING and enqueues its 0420 advice
// in saf_queue, in one DB transaction (root CLAUDE.md's "unknown outcome -> reversal" rule).
// Timeout (reason "68") and POS cancellation (reason "17") both go through this one path.
type ReversalQueuer struct {
	pool   *store.Pool
	encKey []byte
}

// NewReversalQueuer builds a ReversalQueuer. encKey encrypts saf_queue.payload_enc (nil disables
// encryption, useful only in tests).
func NewReversalQueuer(pool *store.Pool, encKey []byte) *ReversalQueuer {
	return &ReversalQueuer{pool: pool, encKey: encKey}
}

// Queue builds the 0420 fields from txn (the original transaction being reversed), transitions
// tran_log txn.Status -> REVERSAL_PENDING, and enqueues the advice - all inside one transaction,
// so a crash between the two never leaves a REVERSAL_PENDING transaction with no queued advice.
func (q *ReversalQueuer) Queue(ctx context.Context, txn store.TranLogRow, reasonCode string) error {
	adv, err := reversalAdvice(txn, reasonCode)
	if err != nil {
		return err
	}
	payload, err := encodePayload(q.encKey, adv)
	if err != nil {
		return fmt.Errorf("encode reversal payload: %w", err)
	}

	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reversal tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Only from the state the caller read: a racing second cancellation, or one after the
	// reversal already went out, must not queue a second 0420.
	moved, err := tx.Exec(ctx,
		`UPDATE tran_log SET state = $2 WHERE id = $1 AND state = $3`, txn.ID, statusReversalPending, txn.Status)
	if err != nil {
		return fmt.Errorf("update tran_log to %s: %w", statusReversalPending, err)
	}
	if moved.RowsAffected() == 0 {
		return fmt.Errorf("%s from %s: %w", txn.RRN, txn.Status, store.ErrNotReversible)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tran_state_history (tran_id, from_state, to_state) VALUES ($1, $2, $3)`,
		txn.ID, txn.Status, statusReversalPending); err != nil {
		return fmt.Errorf("record %s->%s: %w", txn.Status, statusReversalPending, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO saf_queue (tran_id, mti, payload_enc) VALUES ($1, '0420', $2)`, txn.ID, payload); err != nil {
		return fmt.Errorf("enqueue 0420: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reversal tx: %w", err)
	}
	return nil
}

// notStoredInAdvice are the fields QueueAdvice never persists: the MAC, recomputed per send, and
// card data (PAN, track 2, PIN block), which never reaches saf_queue (root CLAUDE.md §6.2).
var notStoredInAdvice = map[int]bool{2: true, 35: true, 52: true, 64: true, 128: true}

// QueueAdvice stores an advice whose delivery is unknown (a 0220 completion that timed out or
// whose 0230 failed its MAC) for the worker to repeat as x21 until the issuer's x30: an advice is
// never declined and never reversed (docs/03 §7.4). sent is the message as it went out; its STAN
// and DE 7 are kept, so every repeat is the same message, and its MAC is dropped, since each send
// is MACed afresh. Card data (DE 2, 35, 52) is dropped too: the payload may be stored
// unencrypted, and a completion advice carries none.
func (q *ReversalQueuer) QueueAdvice(ctx context.Context, tranID int64, mti string, sent map[int]string) error {
	fields := make(map[int]string, len(sent))
	for n, v := range sent {
		if !notStoredInAdvice[n] {
			fields[n] = v
		}
	}
	payload, err := encodePayload(q.encKey, advice{Fields: fields})
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", mti, err)
	}
	if _, err := q.pool.Exec(ctx,
		`INSERT INTO saf_queue (tran_id, mti, payload_enc) VALUES ($1, $2, $3)`, tranID, mti, payload); err != nil {
		return fmt.Errorf("enqueue %s: %w", mti, err)
	}
	return nil
}
