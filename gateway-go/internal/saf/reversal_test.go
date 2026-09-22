package saf

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mcn/gateway-go/internal/store"
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

	id, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000123", Type: "PURCHASE", Status: "SENT", Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", NetworkSTAN: "000123"})
	require.NoError(t, err)
	require.NoError(t, tranLog.UpdateStatus(ctx, id, "TIMED_OUT", "", ""))

	q := NewReversalQueuer(pool, nil)
	txn := store.TranLogRow{ID: id, RRN: "626514000123", Type: "PURCHASE", Status: "TIMED_OUT", NetworkSTAN: "000123", Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", CreatedAt: time.Now()}
	require.NoError(t, q.Queue(ctx, txn, "68"))

	got, err := tranLog.Get(ctx, "626514000123")
	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", got.Status)

	pending, _, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "0420", pending[0].MTI)

	fields, err := decodePayload(nil, pending[0].Payload)
	require.NoError(t, err)
	require.Equal(t, "68", fields[39])
}

func TestReversalQueuer_queuesCancellationAsReasonSeventeen__MCN_401_AC5(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()

	id, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000456", Type: "PURCHASE", Status: "SENT", Amount: 5000, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", NetworkSTAN: "000456"})
	require.NoError(t, err)

	q := NewReversalQueuer(pool, nil)
	txn := store.TranLogRow{ID: id, RRN: "626514000456", Type: "PURCHASE", Status: "SENT", NetworkSTAN: "000456", Amount: 5000, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", CreatedAt: time.Now()}
	require.NoError(t, q.Queue(ctx, txn, "17"))

	pending, _, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	fields, err := decodePayload(nil, pending[0].Payload)
	require.NoError(t, err)
	require.Equal(t, "17", fields[39])
}
