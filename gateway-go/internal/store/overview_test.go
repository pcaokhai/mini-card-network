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
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
		require.NoError(t, repo.UpdateStatus(ctx, id, "APPROVED", "00", "123456"))
		require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))
	}
	decline := func(rrn, rc string) {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID})
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
	decline("626514000007", "") // a legacy row with no RC (OVW-G11)

	stats, err := repo.Overview(ctx, now, utcDay(now))
	require.NoError(t, err)

	require.Equal(t, int64(7), stats.TransactionsToday)
	require.InDelta(t, 3.0/7, stats.ApprovalRate, 0.001) // 3 approved / 7 (3 approved + 4 declined)
	require.GreaterOrEqual(t, stats.P99LatencyMs, int64(0))

	byRC := map[string]int64{}
	for _, dr := range stats.DeclineReasons {
		byRC[dr.ResponseCode] = dr.Count
	}
	require.Equal(t, int64(2), byRC["51"])
	_, hasBlank := byRC[""]
	require.False(t, hasBlank, "OVW-G11: a declined row without an RC is never a blank bucket")
	require.Equal(t, int64(1), byRC["62"])
}

func TestTranLogRepository_overview_emptyIsZeroNotError__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)

	stats, err := repo.Overview(context.Background(), time.Now().UTC(), utcDay(time.Now()))
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
		id, err := repo.Insert(ctx, TranLogRow{RRN: "62651400010" + string(rune('0'+i)), Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
		require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))
		_, err = pool.Exec(ctx,
			`UPDATE tran_state_history SET created_at = created_at - make_interval(secs => $2::float8 / 1000)
			 WHERE tran_id = $1 AND to_state = 'SENT'`, id, latencyMs)
		require.NoError(t, err)
	}

	stats, err := repo.Overview(ctx, time.Now().UTC(), utcDay(time.Now()))
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
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		return id
	}
	for _, rrn := range []string{"626514000201", "626514000202", "626514000203"} {
		insert(rrn)
	}
	for _, rrn := range []string{"626514000211", "626514000212"} {
		id := insert(rrn)
		_, err := pool.Exec(ctx, `UPDATE tran_log SET created_at = $2, business_date = $3 WHERE id = $1`, id, yesterdayMidWindow, dayStart.AddDate(0, 0, -1))
		require.NoError(t, err)
	}

	stats, err := repo.Overview(ctx, now, utcDay(now))
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TransactionsToday)
	require.NotNil(t, stats.TransactionsDeltaPct)
	require.InDelta(t, 0.5, *stats.TransactionsDeltaPct, 0.001) // (3 - 2) / 2
}

func TestTranLogRepository_overview_noDeltaWithoutYesterdayBaseline__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)

	stats, err := repo.Overview(context.Background(), time.Now().UTC(), utcDay(time.Now()))
	require.NoError(t, err)
	require.Nil(t, stats.TransactionsDeltaPct)
	require.Equal(t, int64(0), stats.P50LatencyMs)
}

func TestTranLogRepository_overview_throughputIs24DenseBuckets__MCN_306(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC()

	at := func(rrn string, ago time.Duration) {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE tran_log SET created_at = $2 WHERE id = $1`, id, now.Add(-ago))
		require.NoError(t, err)
	}
	at("626514000401", 10*time.Second)  // newest bucket
	at("626514000402", 20*time.Second)  // newest bucket
	at("626514000403", 160*time.Second) // second newest
	at("626514000404", 59*time.Minute)  // oldest bucket
	at("626514000405", 61*time.Minute)  // outside the window

	stats, err := repo.Overview(ctx, now, utcDay(now))
	require.NoError(t, err)

	require.Len(t, stats.Throughput, 24)
	for i := 1; i < 24; i++ {
		require.Equal(t, 150*time.Second, stats.Throughput[i].At.Sub(stats.Throughput[i-1].At), "bucket %d", i)
	}
	require.WithinDuration(t, now.Add(-150*time.Second), stats.Throughput[23].At, time.Millisecond)
	require.InDelta(t, 2.0/150, stats.Throughput[23].TPS, 1e-9)
	require.InDelta(t, 1.0/150, stats.Throughput[22].TPS, 1e-9)
	require.InDelta(t, 1.0/150, stats.Throughput[0].TPS, 1e-9)
	require.Zero(t, stats.Throughput[10].TPS, "empty buckets are present as zero")
}

// utcDay is a business day that opens at UTC midnight, matching the CURRENT_DATE rows inserted
// without a business date get.
func utcDay(now time.Time) BusinessDay {
	d := now.UTC().Truncate(24 * time.Hour)
	return BusinessDay{Date: d, Previous: d.AddDate(0, 0, -1), OpenedAt: d, PreviousOpenedAt: d.AddDate(0, 0, -1)}
}

func TestTranLogRepository_overview_countsTheBusinessDateNotTheUTCDay__OVW_G7(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	// 01:00 local (UTC+7) on 26 Sep, one hour after the 25th's 23:59:59 cutover; still the 25th in UTC.
	now := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)
	today, previous := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	biz := BusinessDay{
		Date: today, Previous: previous,
		OpenedAt: time.Date(2026, 9, 25, 16, 59, 59, 0, time.UTC), PreviousOpenedAt: time.Date(2026, 9, 24, 16, 59, 59, 0, time.UTC),
	}
	insert := func(rrn string, businessDate, createdAt time.Time, status, rc string) {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: status, ResponseCode: rc, Amount: 1000, Currency: "704",
			MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID, BusinessDate: businessDate})
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE tran_log SET created_at = $2 WHERE id = $1`, id, createdAt)
		require.NoError(t, err)
	}
	insert("626514000601", today, now.Add(-10*time.Minute), statusApproved, "00")
	insert("626514000602", today, now.Add(-5*time.Minute), "DECLINED", "51")
	insert("626514000603", previous, now.Add(-2*time.Hour), "DECLINED", "61")                              // same UTC day, before cutover
	insert("626514000604", previous, time.Date(2026, 9, 24, 17, 30, 0, 0, time.UTC), statusApproved, "00") // 00:30 into the 25th

	stats, err := repo.Overview(ctx, now, biz)

	require.NoError(t, err)
	require.Equal(t, int64(2), stats.TransactionsToday)
	require.InDelta(t, 0.5, stats.ApprovalRate, 0.001)
	require.Len(t, stats.DeclineReasons, 1)
	require.Equal(t, "51", stats.DeclineReasons[0].ResponseCode)
	require.NotNil(t, stats.TransactionsDeltaPct)
	require.InDelta(t, 1.0, *stats.TransactionsDeltaPct, 0.001, "(2 - 1) / 1: only the 25th's first hour counts")

	got, err := repo.Get(ctx, "626514000601")
	require.NoError(t, err)
	require.Equal(t, today, got.BusinessDate, "the stored business date is the calendar's, not CURRENT_DATE")
}
