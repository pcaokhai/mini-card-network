package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTranLogRepository_overview_countsApprovalRateAndDeclineReasons__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC()

	approve := func(rrn string) {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
		require.NoError(t, repo.UpdateStatus(ctx, id, "APPROVED", "00", "123456"))
		require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))
	}
	decline := func(rrn, rc string) {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
		require.NoError(t, repo.UpdateStatus(ctx, id, "DECLINED", rc, ""))
		require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "DECLINED"))
	}

	approve("626514000001")
	approve("626514000002")
	approve("626514000003")
	decline("626514000004", "51")
	decline("626514000005", "51")
	decline("626514000006", "62")

	stats, err := repo.Overview(ctx, now)
	require.NoError(t, err)

	require.Equal(t, int64(6), stats.TransactionsToday)
	require.InDelta(t, 0.5, stats.ApprovalRate, 0.001) // 3 approved / 6 (3 approved + 3 declined)
	require.GreaterOrEqual(t, stats.P99LatencyMs, int64(0))

	byRC := map[string]int64{}
	for _, dr := range stats.DeclineReasons {
		byRC[dr.ResponseCode] = dr.Count
	}
	require.Equal(t, int64(2), byRC["51"])
	require.Equal(t, int64(1), byRC["62"])
}

func TestTranLogRepository_overview_emptyIsZeroNotError__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)

	stats, err := repo.Overview(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(0), stats.TransactionsToday)
	require.Equal(t, float64(0), stats.ApprovalRate)
	require.Empty(t, stats.DeclineReasons)
}
