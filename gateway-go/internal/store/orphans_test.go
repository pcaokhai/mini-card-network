package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const stateSent = "SENT"

func TestTranLogRepository_findsRowsWhoseOutcomeWasNeverRecorded__POS_G16(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	saf := NewSafRepository(pool)
	ctx := context.Background()
	old, recent := time.Now().Add(-5*time.Minute), time.Now()
	insertRC := func(rrn, tranType, status, rc string, sentAt time.Time) int64 {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranType, Status: status, ResponseCode: rc, Amount: 1000, Currency: "704",
			TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: rrn[6:], SentAt: &sentAt})
		require.NoError(t, err)
		return id
	}
	insert := func(rrn, tranType, status string, sentAt time.Time) int64 {
		return insertRC(rrn, tranType, status, "", sentAt)
	}
	insert("626514001001", tranTypePurchase, stateSent, old)             // orphan: never answered
	insert("626514001002", tranTypePurchase, stateSent, recent)          // still in flight
	insert("626514001003", tranTypePurchase, "TIMED_OUT", old)           // orphan: reversal never queued
	queued := insert("626514001004", tranTypePurchase, "TIMED_OUT", old) // its reversal is queued
	_, err := saf.Enqueue(ctx, queued, "0420", []byte("payload"))
	require.NoError(t, err)
	insert("626514001005", "BALANCE", "TIMED_OUT", old) // owes nothing once TIMED_OUT
	insert("626514001006", "BALANCE", stateSent, old)   // orphan: still SENT
	insert("626514001007", tranTypePurchase, statusApproved, old)
	insertRC("626514001008", "PREAUTH", "DECLINED", "96", old) // orphan: bad MAC, its reason-06 0420 never queued
	macQueued := insertRC("626514001009", tranTypePurchase, "DECLINED", "96", old)
	_, err = saf.Enqueue(ctx, macQueued, "0420", []byte("payload"))
	require.NoError(t, err)
	insertRC("626514001010", tranTypePurchase, "DECLINED", "51", old) // a plain decline holds nothing
	insertRC("626514001011", "BALANCE", "DECLINED", "96", old)        // a balance inquiry holds nothing

	orphans, err := repo.FindOrphans(ctx, time.Now().Add(-time.Minute), 10)

	require.NoError(t, err)
	var rrns []string
	for _, o := range orphans {
		rrns = append(rrns, o.RRN)
	}
	require.Equal(t, []string{"626514001001", "626514001003", "626514001006", "626514001008"}, rrns)
}

func TestTranLogRepository_markTimedOutMovesOnlyASentRow__POS_G16(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	id, err := repo.Insert(ctx, TranLogRow{RRN: "626514001011", Type: tranTypePurchase, Status: stateSent, Amount: 1000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID})
	require.NoError(t, err)

	moved, err := repo.MarkTimedOut(ctx, id)
	require.NoError(t, err)
	require.True(t, moved)
	moved, err = repo.MarkTimedOut(ctx, id)
	require.NoError(t, err)
	require.False(t, moved, "a row that already left SENT is left alone")

	history, err := repo.ListStateHistory(ctx, id)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "TIMED_OUT", history[0].ToStatus)
}

func TestTranLogRepository_anOutcomeNeverOverwritesARowTheSweeperMoved__N1(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	id, err := repo.Insert(ctx, TranLogRow{RRN: "626514001021", Type: tranTypePurchase, Status: stateSent, Amount: 1000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID})
	require.NoError(t, err)
	moved, err := repo.MarkTimedOut(ctx, id)
	require.NoError(t, err)
	require.True(t, moved)

	err = repo.UpdateStatusFrom(ctx, id, stateSent, statusApproved, "00", "123456")

	require.ErrorIs(t, err, ErrStateMoved)
	got, err := repo.Get(ctx, "626514001021")
	require.NoError(t, err)
	require.Equal(t, "TIMED_OUT", got.Status, "the stalled request's late answer doesn't undo the sweep")
}
