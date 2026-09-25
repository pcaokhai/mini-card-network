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
