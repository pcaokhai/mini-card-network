package store

import (
	"context"
	"time"
)

const (
	throughputWindowMinutes = 30
	stateApproved           = "APPROVED"
	stateDeclined           = "DECLINED"
	stateTimedOut           = "TIMED_OUT"
)

// OverviewStats is the raw aggregate gateway-go computes from its own tran_log/
// tran_state_history data (contracts/openapi.yaml's Overview schema). Human-readable decline
// labels are added by internal/api, not here - the store stays presentation-free.
type OverviewStats struct {
	TransactionsToday int64
	// TransactionsDeltaPct compares today with the same elapsed window yesterday; nil when
	// yesterday had no transactions in that window, since a ratio against zero means nothing.
	TransactionsDeltaPct *float64
	ApprovalRate         float64
	P50LatencyMs         int64
	P99LatencyMs         int64
	Throughput           []ThroughputSample
	DeclineReasons       []DeclineReasonCount
}

// ThroughputSample is one bucket in the throughput chart.
type ThroughputSample struct {
	At  time.Time
	TPS float64
}

// DeclineReasonCount is a raw response-code tally; internal/api attaches the easy-text label.
type DeclineReasonCount struct {
	ResponseCode string
	Count        int64
}

// Overview computes today's KPIs as of now. "Today" is a UTC calendar-day boundary anchored to
// now - a dashboard read, not the financial business-date cutover other parts of this codebase
// use for settlement.
func (r *TranLogRepository) Overview(ctx context.Context, now time.Time) (OverviewStats, error) {
	dayStart := now.Truncate(24 * time.Hour)

	stats, err := r.overviewCounts(ctx, dayStart)
	if err != nil {
		return OverviewStats{}, err
	}
	if stats.TransactionsDeltaPct, err = r.overviewDeltaPct(ctx, now, dayStart, stats.TransactionsToday); err != nil {
		return OverviewStats{}, err
	}
	if stats.P50LatencyMs, stats.P99LatencyMs, err = r.overviewLatencyPercentiles(ctx, dayStart); err != nil {
		return OverviewStats{}, err
	}
	if stats.Throughput, err = r.overviewThroughput(ctx, now); err != nil {
		return OverviewStats{}, err
	}
	if stats.DeclineReasons, err = r.overviewDeclineReasons(ctx, dayStart); err != nil {
		return OverviewStats{}, err
	}
	return stats, nil
}

func (r *TranLogRepository) overviewCounts(ctx context.Context, dayStart time.Time) (OverviewStats, error) {
	var stats OverviewStats
	var approved, declined int64
	err := r.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE created_at >= $1),
			count(*) FILTER (WHERE created_at >= $1 AND state = $2),
			count(*) FILTER (WHERE created_at >= $1 AND state = $3)
		FROM tran_log
	`, dayStart, stateApproved, stateDeclined).Scan(&stats.TransactionsToday, &approved, &declined)
	if err != nil {
		return OverviewStats{}, err
	}
	if approved+declined > 0 {
		stats.ApprovalRate = float64(approved) / float64(approved+declined)
	}
	return stats, nil
}

// overviewLatencyPercentiles returns the p50 and p99 time from SENT to the first terminal state
// (APPROVED/DECLINED/TIMED_OUT), in milliseconds, over today's transactions.
func (r *TranLogRepository) overviewLatencyPercentiles(ctx context.Context, dayStart time.Time) (int64, int64, error) {
	var p50, p99 float64
	err := r.pool.QueryRow(ctx, `
		WITH latency AS (
			SELECT EXTRACT(EPOCH FROM (terminal.created_at - sent.created_at)) * 1000 AS ms
			FROM tran_state_history sent
			JOIN tran_state_history terminal
				ON terminal.tran_id = sent.tran_id AND terminal.to_state IN ($2, $3, $4)
			WHERE sent.to_state = 'SENT' AND sent.created_at >= $1
		)
		SELECT
			coalesce(percentile_cont(0.50) WITHIN GROUP (ORDER BY ms), 0),
			coalesce(percentile_cont(0.99) WITHIN GROUP (ORDER BY ms), 0)
		FROM latency
	`, dayStart, stateApproved, stateDeclined, stateTimedOut).Scan(&p50, &p99)
	if err != nil {
		return 0, 0, err
	}
	return int64(p50), int64(p99), nil
}

// overviewDeltaPct compares today's count with yesterday's over the same elapsed time, so a
// morning read is not measured against yesterday's full day.
func (r *TranLogRepository) overviewDeltaPct(ctx context.Context, now, dayStart time.Time, today int64) (*float64, error) {
	var yesterday int64
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM tran_log WHERE created_at >= $1 AND created_at <= $2`,
		dayStart.Add(-24*time.Hour), now.Add(-24*time.Hour)).Scan(&yesterday)
	if err != nil {
		return nil, err
	}
	if yesterday == 0 {
		return nil, nil
	}
	delta := float64(today-yesterday) / float64(yesterday)
	return &delta, nil
}

func (r *TranLogRepository) overviewThroughput(ctx context.Context, now time.Time) ([]ThroughputSample, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date_trunc('minute', created_at) AS bucket, count(*)
		FROM tran_log
		WHERE created_at >= $1
		GROUP BY bucket
		ORDER BY bucket
	`, now.Add(-throughputWindowMinutes*time.Minute))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []ThroughputSample
	for rows.Next() {
		var at time.Time
		var count int64
		if err := rows.Scan(&at, &count); err != nil {
			return nil, err
		}
		samples = append(samples, ThroughputSample{At: at, TPS: float64(count) / 60})
	}
	return samples, rows.Err()
}

func (r *TranLogRepository) overviewDeclineReasons(ctx context.Context, dayStart time.Time) ([]DeclineReasonCount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT coalesce(response_code, ''), count(*)
		FROM tran_log
		WHERE created_at >= $1 AND state = $2
		GROUP BY response_code
		ORDER BY count(*) DESC
	`, dayStart, stateDeclined)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reasons []DeclineReasonCount
	for rows.Next() {
		var reason DeclineReasonCount
		if err := rows.Scan(&reason.ResponseCode, &reason.Count); err != nil {
			return nil, err
		}
		reasons = append(reasons, reason)
	}
	return reasons, rows.Err()
}
