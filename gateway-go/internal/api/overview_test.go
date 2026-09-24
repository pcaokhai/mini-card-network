package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeOverviewReader struct {
	stats store.OverviewStats
}

func (f *fakeOverviewReader) Overview(context.Context, time.Time) (store.OverviewStats, error) {
	return f.stats, nil
}

func TestGetOverview_returnsKpisAndDeclineLabels__MCN_306(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeOverviewReader{stats: store.OverviewStats{
		TransactionsToday: 6,
		ApprovalRate:      0.5,
		P99LatencyMs:      42,
		Throughput:        []store.ThroughputSample{{At: time.Unix(0, 0).UTC(), TPS: 2.5}},
		DeclineReasons:    []store.DeclineReasonCount{{ResponseCode: "51", Count: 2}, {ResponseCode: "62", Count: 1}},
	}}
	MountOverview(r, reader)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		TransactionsToday int64   `json:"transactionsToday"`
		ApprovalRate      float64 `json:"approvalRate"`
		P99LatencyMs      int64   `json:"p99LatencyMs"`
		LedgerMatches     bool    `json:"ledgerMatches"`
		Throughput        []struct {
			At  time.Time `json:"at"`
			TPS float64   `json:"tps"`
		} `json:"throughput"`
		DeclineReasons []struct {
			ResponseCode string  `json:"responseCode"`
			Label        string  `json:"label"`
			Share        float64 `json:"share"`
		} `json:"declineReasons"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	require.Equal(t, int64(6), body.TransactionsToday)
	require.InDelta(t, 0.5, body.ApprovalRate, 0.001)
	require.Equal(t, int64(42), body.P99LatencyMs)
	require.True(t, body.LedgerMatches)
	require.Len(t, body.Throughput, 1)
	require.InDelta(t, 2.5, body.Throughput[0].TPS, 0.001)

	require.Len(t, body.DeclineReasons, 2)
	require.Equal(t, "51", body.DeclineReasons[0].ResponseCode)
	require.Equal(t, "Insufficient funds", body.DeclineReasons[0].Label)
	require.InDelta(t, 2.0/3.0, body.DeclineReasons[0].Share, 0.001)
	require.Equal(t, "62", body.DeclineReasons[1].ResponseCode)
	require.Equal(t, "Card is blocked", body.DeclineReasons[1].Label)
	require.InDelta(t, 1.0/3.0, body.DeclineReasons[1].Share, 0.001)
}

func TestGetOverview_emptyDeclineReasonsHasNoShareDivideByZero__MCN_306(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeOverviewReader{stats: store.OverviewStats{}}
	MountOverview(r, reader)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"declineReasons":[]`)
}
