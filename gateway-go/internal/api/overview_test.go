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
	day   store.BusinessDay
	err   error
}

func (f *fakeOverviewReader) Overview(_ context.Context, _ time.Time, day store.BusinessDay) (store.OverviewStats, error) {
	f.day = day
	return f.stats, f.err
}

// fixedCalendar is a BusinessCalendar whose business date never moves.
type fixedCalendar struct{ date time.Time }

func (c fixedCalendar) Current(time.Time) time.Time    { return c.date }
func (c fixedCalendar) Previous(d time.Time) time.Time { return d.AddDate(0, 0, -1) }
func (c fixedCalendar) OpenedAt(d time.Time) time.Time { return d.Add(-7 * time.Hour) }

var testCalendar = fixedCalendar{date: time.Date(2031, 12, 31, 0, 0, 0, 0, time.UTC)}

func TestGetOverview_countsTheCurrentBusinessDate__OVW_G7(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeOverviewReader{}
	MountOverview(r, reader, testCalendar)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/v1/metrics/overview", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"businessDate":"2031-12-31"`)
	require.Equal(t, store.BusinessDay{
		Date: testCalendar.date, Previous: time.Date(2031, 12, 30, 0, 0, 0, 0, time.UTC),
		OpenedAt: time.Date(2031, 12, 30, 17, 0, 0, 0, time.UTC), PreviousOpenedAt: time.Date(2031, 12, 29, 17, 0, 0, 0, time.UTC),
	}, reader.day)
	requireMatchesSpec(t, req, rec)
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
	MountOverview(r, reader, testCalendar)

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
	MountOverview(r, reader, testCalendar)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"declineReasons":[]`)
}

func TestGetOverview_exposesP50AndDeltaWhenKnown__MCN_306(t *testing.T) {
	delta := 0.12
	r := chi.NewRouter()
	MountOverview(r, &fakeOverviewReader{stats: store.OverviewStats{P50LatencyMs: 96, P99LatencyMs: 212, TransactionsDeltaPct: &delta}}, testCalendar)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.InDelta(t, 96, body["p50LatencyMs"], 0.001)
	require.InDelta(t, 0.12, body["transactionsDeltaPct"], 0.001)
}

func TestGetOverview_omitsDeltaWithoutBaseline__MCN_306(t *testing.T) {
	r := chi.NewRouter()
	MountOverview(r, &fakeOverviewReader{stats: store.OverviewStats{}}, testCalendar)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/metrics/overview", nil))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotContains(t, body, "transactionsDeltaPct")
}
