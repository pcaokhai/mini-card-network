package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func insertTestTran(ctx context.Context, t *testing.T, tranLog *TranLogRepository, rrn, stan string) int64 {
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

	tranID := insertTestTran(ctx, t, tranLog, "626514000123", "000123")

	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("encrypted-0420-payload"))
	require.NoError(t, err)

	claimed, err := saf.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, id, claimed[0].ID)
	require.Equal(t, 0, claimed[0].Attempts)

	require.NoError(t, saf.MarkInFlight(ctx, id, 1, time.Now().Add(2*time.Second), "i/o timeout"))
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

	tranID := insertTestTran(ctx, t, tranLog, "626514000456", "000456")
	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT id FROM saf_queue WHERE id = $1 FOR UPDATE`, id)
	require.NoError(t, err)

	claimed, err := saf.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, claimed) // the row is locked by tx above, SKIP LOCKED must skip it
}

func TestSafRepository_markDeadIncrementsDeadCount__MCN_401_AC3(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(ctx, t, tranLog, "626514000789", "000789")
	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)

	require.NoError(t, saf.MarkDead(ctx, id, "max attempts exceeded"))

	_, deadCount, err := saf.ListPending(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deadCount)
}

func TestSafRepository_listItemsReportsRRNAndDeadCount__MCN_407(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(ctx, t, tranLog, "626514000999", "000999")
	pendingID, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)
	deadTranID := insertTestTran(ctx, t, tranLog, "626514001111", "001111")
	deadID, err := saf.Enqueue(ctx, deadTranID, "0420", []byte("payload"))
	require.NoError(t, err)
	require.NoError(t, saf.MarkDead(ctx, deadID, "max attempts exceeded"))

	snap, err := saf.ListItems(ctx)

	require.NoError(t, err)
	require.Equal(t, 1, snap.DeadCount)
	require.Equal(t, 1, snap.Depth) // PENDING + IN_FLIGHT only (NET-G7)
	items := snap.Items
	require.Len(t, items, 2)
	require.Equal(t, pendingID, items[0].ID) // owed advices first, DEAD after
	byID := map[int64]SafItemRow{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.Equal(t, "626514000999", byID[pendingID].RRN)
	require.Equal(t, int64(10000), byID[pendingID].AmountMinor)
	require.Equal(t, "PENDING", byID[pendingID].Status)
	require.Equal(t, "DEAD", byID[deadID].Status)
}

func TestSafRepository_ackOf0420CompletesTheReversal__MCN_002(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(ctx, t, tranLog, "626514000501", "000501")
	require.NoError(t, tranLog.UpdateStatus(ctx, tranID, "REVERSAL_PENDING", "", ""))
	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)

	require.NoError(t, saf.MarkAcked(ctx, id))

	row, err := tranLog.Get(ctx, "626514000501")
	require.NoError(t, err)
	require.Equal(t, "REVERSED", row.Status)
	history, err := tranLog.ListStateHistory(ctx, tranID)
	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", history[len(history)-1].FromStatus)
	require.Equal(t, "REVERSED", history[len(history)-1].ToStatus)
	rrn, err := saf.TranRRN(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "626514000501", rrn, "OVW-G3: the acknowledged row can be announced")
}

func TestSafRepository_ackOfAnAdviceLeavesTheTransactionState__MCN_002(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()

	tranID := insertTestTran(ctx, t, tranLog, "626514000502", "000502")
	require.NoError(t, tranLog.UpdateStatus(ctx, tranID, "APPROVED", "00", "123456"))
	id, err := saf.Enqueue(ctx, tranID, "0220", []byte("payload"))
	require.NoError(t, err)

	require.NoError(t, saf.MarkAcked(ctx, id))

	row, err := tranLog.Get(ctx, "626514000502")
	require.NoError(t, err)
	require.Equal(t, "APPROVED", row.Status)
}

// A claimed row is leased: until the lease runs out no claim picks it up again, so a row whose
// ACK write failed is not resent at once.
func TestSafRepository_aClaimedRowIsLeased__MCN_401(t *testing.T) {
	pool := newTestPool(t)
	saf := NewSafRepository(pool)
	ctx := context.Background()
	tranID := insertTestTran(ctx, t, NewTranLogRepository(pool), "626514000321", "000321")
	_, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
	require.NoError(t, err)

	first, err := saf.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)
	again, err := saf.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Empty(t, again)
}

func TestSafRepository_findReversalReturnsLatest0420__MCN_304(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()
	tranID := insertTestTran(ctx, t, tranLog, "626514000789", "000789")
	other := insertTestTran(ctx, t, tranLog, "626514000790", "000790")

	_, err := saf.FindReversal(ctx, tranID)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = saf.Enqueue(ctx, tranID, "0420", []byte("first"))
	require.NoError(t, err)
	id, err := saf.Enqueue(ctx, tranID, "0420", []byte("second"))
	require.NoError(t, err)
	_, err = saf.Enqueue(ctx, other, "0420", []byte("other"))
	require.NoError(t, err)
	require.NoError(t, saf.MarkInFlight(ctx, id, 2, time.Now(), "i/o timeout"))
	require.NoError(t, saf.MarkAcked(ctx, id))

	got, err := saf.FindReversal(ctx, tranID)
	require.NoError(t, err)

	require.Equal(t, id, got.ID)
	require.Equal(t, []byte("second"), got.Payload)
	require.Equal(t, "ACKED", got.Status)
	require.Equal(t, 2, got.Attempts)
	require.False(t, got.CreatedAt.IsZero())
	require.NotNil(t, got.AckedAt)
}

func TestSafRepository_listItemsCapsItemsButCountsEverything__NET_G7(t *testing.T) {
	pool := newTestPool(t)
	tranLog := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()
	for i := 0; i < SafItemsCap+3; i++ {
		stan := fmt.Sprintf("%06d", 100000+i)
		tranID := insertTestTran(ctx, t, tranLog, "6265141"+stan[1:], stan)
		_, err := saf.Enqueue(ctx, tranID, "0420", []byte("payload"))
		require.NoError(t, err)
	}

	snap, err := saf.ListItems(ctx)

	require.NoError(t, err)
	require.Len(t, snap.Items, SafItemsCap)
	require.Equal(t, SafItemsCap+3, snap.Depth)
}
