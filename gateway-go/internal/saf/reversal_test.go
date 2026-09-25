package saf

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	testMerchantID   = "GOCPHO000000001"
	tranTypePurchase = "PURCHASE"
	statusSent       = "SENT"
	statusTimedOut   = "TIMED_OUT"
)

func newTestPool(t *testing.T) *store.Pool {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("acquirer"), postgres.WithUsername("acquirer"), postgres.WithPassword("test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, store.Migrate(dsn))
	pool, err := store.Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestReversalQueuer_queuesTimeoutAsReasonSixtyEightAtomically__MCN_401_AC1(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()

	id, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000123", Type: tranTypePurchase, Status: statusSent, Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000123"})
	require.NoError(t, err)
	require.NoError(t, tranLog.UpdateStatus(ctx, id, "TIMED_OUT", "", ""))

	q := NewReversalQueuer(pool, nil)
	sentAt := time.Now().UTC()
	txn := store.TranLogRow{ID: id, RRN: "626514000123", Type: tranTypePurchase, Status: "TIMED_OUT", NetworkSTAN: "000123", Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, CreatedAt: time.Now(),
		ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: testCardToken}
	require.NoError(t, q.Queue(ctx, txn, "68"))

	got, err := tranLog.Get(ctx, "626514000123")
	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", got.Status)

	pending, _, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "0420", pending[0].MTI)

	adv, err := decodePayload(nil, pending[0].Payload)
	require.NoError(t, err)
	require.Equal(t, "68", adv.Fields[39])
	require.Equal(t, testCardToken, adv.CardToken)
}

func TestReversalQueuer_queuesCancellationAsReasonSeventeen__MCN_401_AC5(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()

	id, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000456", Type: tranTypePurchase, Status: statusSent, Amount: 5000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000456"})
	require.NoError(t, err)

	q := NewReversalQueuer(pool, nil)
	sentAt := time.Now().UTC()
	txn := store.TranLogRow{ID: id, RRN: "626514000456", Type: tranTypePurchase, Status: statusSent, NetworkSTAN: "000456", Amount: 5000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, CreatedAt: time.Now(),
		ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: testCardToken}
	require.NoError(t, q.Queue(ctx, txn, "17"))

	pending, _, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	adv, err := decodePayload(nil, pending[0].Payload)
	require.NoError(t, err)
	require.Equal(t, "17", adv.Fields[39])
}

// A cancelled purchase, from the queued 0420 through the 0430, ends REVERSED on the real schema,
// using what the purchase itself recorded (not a hand-built original).
func TestReversal_cancellationIsDeliveredAndCompletes__MCN_401(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()
	sentAt := time.Now().UTC().Truncate(time.Second)
	_, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000789", Type: tranTypePurchase, Status: "APPROVED", Amount: 71000, Currency: "704",
		MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000789",
		ProcessingCode: "000000", POSEntryMode: "052", SentAt: &sentAt, CardToken: testCardToken})
	require.NoError(t, err)
	original, err := tranLog.Get(ctx, "626514000789")
	require.NoError(t, err)

	require.NoError(t, NewReversalQueuer(pool, nil).Queue(ctx, original, "17"))
	mux := &fakeMux{response: map[int]string{39: "00"}}
	w := NewWorker(mux, fakeCards{testCardToken: testPAN}, &recordingHSM{}, make([]byte, 16), safRepo, nil,
		isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)
	require.NoError(t, w.deliverOnce(ctx))

	sent := mux.sentFields[0]
	require.Equal(t, "0200000789"+sentAt.Format("0102150405")+"0000097049900000000000", sent[90], "DE 90 names the purchase exactly as sent")
	got, err := tranLog.Get(ctx, "626514000789")
	require.NoError(t, err)
	require.Equal(t, "REVERSED", got.Status)
}

// Queue moves the transaction only from the state its caller read: a second cancellation racing
// the first, or one arriving after the reversal completed, queues nothing.
func TestReversalQueuer_refusesATransactionWhoseStateMovedOn__MCN_401(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()
	sentAt := time.Now().UTC()
	_, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000654", Type: tranTypePurchase, Status: "APPROVED", Amount: 5000, Currency: "704",
		TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000654", ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: testCardToken})
	require.NoError(t, err)
	original, err := tranLog.Get(ctx, "626514000654")
	require.NoError(t, err)
	q := NewReversalQueuer(pool, nil)

	require.NoError(t, q.Queue(ctx, original, "17"))
	err = q.Queue(ctx, original, "17") // still reads APPROVED; the row is REVERSAL_PENDING now

	require.ErrorIs(t, err, store.ErrNotReversible)
	pending, _, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

// A timed-out pre-auth reverses from the state its row really holds, naming the 0100 in DE 90; a
// caller that passes a stale state queues nothing.
func TestReversalQueuer_reversesAPreAuthFromItsCurrentState__POS_G4(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()
	sentAt := time.Now().UTC()
	id, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000321", Type: "PREAUTH", Status: statusTimedOut, Amount: 5000, Currency: "704", MTI: "0100",
		TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000321", ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: testCardToken})
	require.NoError(t, err)
	row, err := tranLog.Get(ctx, "626514000321")
	require.NoError(t, err)
	q := NewReversalQueuer(pool, nil)

	stale := row
	stale.Status = statusSent
	require.ErrorIs(t, q.Queue(ctx, stale, "68"), store.ErrNotReversible)
	require.NoError(t, q.Queue(ctx, row, "68"))

	pending, _, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	adv, err := decodePayload(nil, pending[0].Payload)
	require.NoError(t, err)
	require.Equal(t, "0100000321", adv.Fields[90][:10])
	history, err := tranLog.ListStateHistory(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", history[len(history)-1].ToStatus)
}

// An unknown-outcome completion is never reversed: its 0220 itself is stored and repeated as 0221
// until the 0230 (docs/03 §7.4), with no DE 2 and a DE 64 MAC, and the ACK approves the row.
func TestAdviceQueuer_repeatsAnUnknownCompletionUntilThe0230__POS_G4(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()
	id, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000322", Type: "COMPLETION", Status: statusTimedOut, Amount: 5000, Currency: "704", MTI: "0220",
		TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000322"})
	require.NoError(t, err)
	sent := map[int]string{3: "000000", 4: "000000005000", 7: "0925101500", 11: "000322", 32: "970499", 37: "626514000300", 49: "704", 64: "0102030405060708"}

	require.NoError(t, NewReversalQueuer(pool, nil).QueueAdvice(ctx, id, "0220", sent))
	mux := &fakeMux{response: map[int]string{39: "00"}}
	w := NewWorker(mux, fakeCards{}, &recordingHSM{}, make([]byte, 16), safRepo, nil,
		isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)
	require.NoError(t, w.deliverOnce(ctx))

	require.Equal(t, []string{"0221"}, mux.sentMTIs, "the 0220 may already be at the issuer")
	frame := mux.sentFields[0]
	require.Equal(t, "000322", frame[11], "the same message, only the MTI changed")
	require.Equal(t, "0925101500", frame[7])
	require.NotContains(t, frame, 2, "a completion carries no PAN")
	require.NotContains(t, frame, 128)
	require.NotEqual(t, "0102030405060708", frame[64], "MACed afresh for the 0221")
	got, err := tranLog.Get(ctx, "626514000322")
	require.NoError(t, err)
	require.Equal(t, "APPROVED", got.Status, "an acknowledged advice was recorded by the issuer")
	require.Equal(t, "00", got.ResponseCode)
}
