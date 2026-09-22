package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeTranLogReader struct {
	page       []store.TranLogRow
	nextCursor string
	byRRN      map[string]store.TranLogRow
	history    []store.StateTransition
}

func (f *fakeTranLogReader) List(context.Context, store.TransactionFilter) ([]store.TranLogRow, string, error) {
	return f.page, f.nextCursor, nil
}

func (f *fakeTranLogReader) Get(_ context.Context, rrn string) (store.TranLogRow, error) {
	row, ok := f.byRRN[rrn]
	if !ok {
		return store.TranLogRow{}, store.ErrNotFound
	}
	return row, nil
}

func (f *fakeTranLogReader) ListStateHistory(context.Context, int64) ([]store.StateTransition, error) {
	return f.history, nil
}

func TestGetTransactions_returnsPaginatedList__MCN_304_AC1(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeTranLogReader{
		page:       []store.TranLogRow{{RRN: "a", Type: "PURCHASE", Status: "APPROVED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantName: "Ca phe Goc Pho", CreatedAt: time.Now()}},
		nextCursor: "abc",
	}
	MountTransactionsQuery(r, reader)

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions?limit=1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"rrn":"a"`)
	require.Contains(t, rec.Body.String(), `"nextCursor":"abc"`)
}

func TestGetTransaction_returns404ForUnknownRrn__MCN_304_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{}})

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/nope", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetTransaction_returnsTransaction__MCN_304_AC1(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeTranLogReader{byRRN: map[string]store.TranLogRow{
		"x": {RRN: "x", Type: "PURCHASE", Status: "APPROVED", ResponseCode: "00", AuthCode: "123456", Amount: 5000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantName: "Ca phe Goc Pho", CreatedAt: time.Now()},
	}}
	MountTransactionsQuery(r, reader)

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/x", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"authCode":"123456"`)
}

func TestGetTransactionJourney_returnsStepsAndMoney__MCN_304_AC2(t *testing.T) {
	now := time.Now()
	r := chi.NewRouter()
	reader := &fakeTranLogReader{
		byRRN: map[string]store.TranLogRow{
			"x": {ID: 1, RRN: "x", Type: "PURCHASE", Status: "APPROVED", ResponseCode: "00", Amount: 10000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantName: "Ca phe Goc Pho", CreatedAt: now},
		},
		history: []store.StateTransition{
			{FromStatus: "CREATED", ToStatus: "SENT", At: now},
			{FromStatus: "SENT", ToStatus: "APPROVED", At: now.Add(50 * time.Millisecond)},
		},
	}
	MountTransactionsQuery(r, reader)

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/x/journey", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"steps"`)
	require.Contains(t, rec.Body.String(), `"money"`)
	require.Contains(t, rec.Body.String(), "Approved")
}

func TestGetTransactionJourney_returns404ForUnknownRrn__MCN_304_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{}})

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/nope/journey", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
