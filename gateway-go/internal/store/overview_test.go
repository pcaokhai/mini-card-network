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

func TestTranLogRepository_overview_p50AndP99FromControlledLatencies__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	for i, latencyMs := range []int{100, 200, 300, 1000} {
		id, err := repo.Insert(ctx, TranLogRow{RRN: "62651400010" + string(rune('0'+i)), Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
		require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))
		_, err = pool.Exec(ctx,
			`UPDATE tran_state_history SET created_at = created_at - make_interval(secs => $2::float8 / 1000)
			 WHERE tran_id = $1 AND to_state = 'SENT'`, id, latencyMs)
		require.NoError(t, err)
	}

	stats, err := repo.Overview(ctx, time.Now().UTC())
	require.NoError(t, err)
	// percentile_cont interpolates: median of {100,200,300,1000} is 250; p99 is 300 + 0.97*700.
	require.InDelta(t, 250, stats.P50LatencyMs, 25)
	require.InDelta(t, 979, stats.P99LatencyMs, 25)
}

func TestTranLogRepository_overview_deltaVersusSameWindowYesterday__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	yesterdayMidWindow := dayStart.Add(-24 * time.Hour).Add(now.Sub(dayStart) / 2)

	insert := func(rrn string) int64 {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		return id
	}
	for _, rrn := range []string{"626514000201", "626514000202", "626514000203"} {
		insert(rrn)
	}
	for _, rrn := range []string{"626514000211", "626514000212"} {
		id := insert(rrn)
		_, err := pool.Exec(ctx, `UPDATE tran_log SET created_at = $2 WHERE id = $1`, id, yesterdayMidWindow)
		require.NoError(t, err)
	}

	stats, err := repo.Overview(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TransactionsToday)
	require.NotNil(t, stats.TransactionsDeltaPct)
	require.InDelta(t, 0.5, *stats.TransactionsDeltaPct, 0.001) // (3 - 2) / 2
}

func TestTranLogRepository_overview_noDeltaWithoutYesterdayBaseline__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)

	stats, err := repo.Overview(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Nil(t, stats.TransactionsDeltaPct)
	require.Equal(t, int64(0), stats.P50LatencyMs)
}
