package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/store"
)

// OverviewReader is the read-side port overview.go needs; *store.TranLogRepository satisfies it.
type OverviewReader interface {
	Overview(ctx context.Context, now time.Time) (store.OverviewStats, error)
}

// MountOverview registers GET /v1/metrics/overview (contracts/openapi.yaml, tag "metrics").
func MountOverview(r chi.Router, reader OverviewReader) {
	r.Get("/v1/metrics/overview", handleGetOverview(reader))
}

type throughputSampleDTO struct {
	At  time.Time `json:"at"`
	TPS float64   `json:"tps"`
}

type declineReasonDTO struct {
	ResponseCode string  `json:"responseCode"`
	Label        string  `json:"label"`
	Share        float64 `json:"share"`
}

type overviewDTO struct {
	TransactionsToday    int64                 `json:"transactionsToday"`
	TransactionsDeltaPct *float64              `json:"transactionsDeltaPct,omitempty"`
	ApprovalRate         float64               `json:"approvalRate"`
	P50LatencyMs         int64                 `json:"p50LatencyMs"`
	P99LatencyMs         int64                 `json:"p99LatencyMs"`
	LedgerMatches        bool                  `json:"ledgerMatches"`
	Throughput           []throughputSampleDTO `json:"throughput"`
	DeclineReasons       []declineReasonDTO    `json:"declineReasons"`
}

func handleGetOverview(reader OverviewReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		stats, err := reader.Overview(req.Context(), time.Now().UTC())
		if err != nil {
			problem(w, http.StatusInternalServerError, "overview-read-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, toOverviewDTO(stats))
	}
}

func toOverviewDTO(stats store.OverviewStats) overviewDTO {
	throughput := make([]throughputSampleDTO, len(stats.Throughput))
	for i, s := range stats.Throughput {
		throughput[i] = throughputSampleDTO{At: s.At, TPS: s.TPS}
	}

	var totalDeclined int64
	for _, dr := range stats.DeclineReasons {
		totalDeclined += dr.Count
	}
	declineReasons := make([]declineReasonDTO, len(stats.DeclineReasons))
	for i, dr := range stats.DeclineReasons {
		var share float64
		if totalDeclined > 0 {
			share = float64(dr.Count) / float64(totalDeclined)
		}
		declineReasons[i] = declineReasonDTO{ResponseCode: dr.ResponseCode, Label: journey.EasyTextForRC(dr.ResponseCode), Share: share}
	}

	return overviewDTO{
		TransactionsToday:    stats.TransactionsToday,
		TransactionsDeltaPct: stats.TransactionsDeltaPct,
		ApprovalRate:         stats.ApprovalRate,
		P50LatencyMs:         stats.P50LatencyMs,
		P99LatencyMs:         stats.P99LatencyMs,
		// ponytail: the gateway doesn't own the ledger (only the issuer does, per
		// docs/02 §4) and has no cross-service balance-reconciliation read yet - same
		// documented limitation as internal/chaos/runner.go's closingBalanceTotal.
		// True until a real ledger-discrepancy check exists.
		LedgerMatches:  true,
		Throughput:     throughput,
		DeclineReasons: declineReasons,
	}
}
