package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func insertTestTran(t *testing.T, ctx context.Context, tranLog *TranLogRepository, rrn, stan string) int64 {
	t.Helper()
	id, err := tranLog.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "SENT", Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: stan})
	require.NoError(t, err)
	return id
}

func TestSafRepository_enqueueClaimAndAck__MCN_401_AC2(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(t, ctx, tranLog, "626514000123", "000123")

	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("encrypted-0420-payload"))
	require.NoError(t, err)

	claimed, err := saf.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, id, claimed[0].ID)
	require.Equal(t, 0, claimed[0].Attempts)

	require.NoError(t, saf.MarkInFlight(ctx, id, 1, time.Now().Add(2*time.Second)))
	require.NoError(t, saf.MarkAcked(ctx, id))

	_, deadCount, err := saf.ListPending(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, deadCount)
}

func TestSafRepository_claimDueSkipsLockedRows__MCN_401_AC2(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(t, ctx, tranLog, "626514000456", "000456")
	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT id FROM saf_queue WHERE id = $1 FOR UPDATE`, id)
	require.NoError(t, err)

	claimed, err := saf.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, claimed) // the row is locked by tx above, SKIP LOCKED must skip it
}

func TestSafRepository_markDeadIncrementsDeadCount__MCN_401_AC3(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(t, ctx, tranLog, "626514000789", "000789")
	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)

	require.NoError(t, saf.MarkDead(ctx, id, "max attempts exceeded"))

	_, deadCount, err := saf.ListPending(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deadCount)
}
