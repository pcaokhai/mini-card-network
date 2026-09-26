package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	testMaskedPAN    = "970436******4417"
	statusApproved   = "APPROVED"
	statusSent       = "SENT"
	testMerchantName = "Ca phe Goc Pho"
	tranTypePurchase = "PURCHASE"
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

type fakeReversals struct{ rev *journey.Reversal }

func (f fakeReversals) Reversal(context.Context, int64) (*journey.Reversal, error) { return f.rev, nil }

func TestGetTransactions_returnsPaginatedList__MCN_304_AC1(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeTranLogReader{
		page:       []store.TranLogRow{{RRN: "a", Type: tranTypePurchase, Status: statusApproved, Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName, CreatedAt: time.Now()}},
		nextCursor: "abc",
	}
	MountTransactionsQuery(r, reader, fakeReversals{})

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions?limit=1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"rrn":"a"`)
	require.Contains(t, rec.Body.String(), `"nextCursor":"abc"`)
}

func TestGetTransaction_returns404ForUnknownRrn__MCN_304_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{}}, fakeReversals{})

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/nope", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetTransaction_returnsTransaction__MCN_304_AC1(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeTranLogReader{byRRN: map[string]store.TranLogRow{
		"x": {RRN: "x", Type: tranTypePurchase, Status: statusApproved, ResponseCode: "00", AuthCode: "123456", Amount: 5000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName, CreatedAt: time.Now()},
	}}
	MountTransactionsQuery(r, reader, fakeReversals{})

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
			"x": {ID: 1, RRN: "x", Type: tranTypePurchase, Status: statusApproved, ResponseCode: "00", Amount: 10000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName, CreatedAt: now},
		},
		history: []store.StateTransition{
			{FromStatus: "CREATED", ToStatus: statusSent, At: now},
			{FromStatus: statusSent, ToStatus: statusApproved, At: now.Add(50 * time.Millisecond)},
		},
	}
	MountTransactionsQuery(r, reader, fakeReversals{})

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
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{}}, fakeReversals{})

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/nope/journey", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func reversedTimeout(now time.Time) (store.TranLogRow, []store.StateTransition, *journey.Reversal) {
	sent, responded, acked := now, now.Add(30*time.Second), now.Add(30090*time.Millisecond)
	row := store.TranLogRow{
		ID: 1, RRN: "626514000124", Type: tranTypePurchase, Status: "REVERSED", Amount: 600000, Currency: "704",
		MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: fixtureMID, MerchantName: testMerchantName,
		NetworkSTAN: "000124", ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sent, RespondedAt: &responded, CreatedAt: now,
	}
	history := []store.StateTransition{
		{FromStatus: "CREATED", ToStatus: statusSent, At: now},
		{FromStatus: statusSent, ToStatus: "TIMED_OUT", At: responded},
		{FromStatus: "TIMED_OUT", ToStatus: statusReversalPending, At: now.Add(30010 * time.Millisecond)},
		{FromStatus: statusReversalPending, ToStatus: "REVERSED", At: acked},
	}
	rev := &journey.Reversal{Status: "ACKED", QueuedAt: now.Add(30010 * time.Millisecond), AckedAt: &acked, Fields: map[int]string{
		3: "000000", 4: "000000600000", 7: now.Add(30020 * time.Millisecond).UTC().Format("0102150405"), 11: "000125",
		37: "626514000124", 39: "68", 41: "00000042", 42: fixtureMID, 49: "704", 90: "0200000124",
	}}
	return row, history, rev
}

func TestGetTransactionJourney_embedsCodesAndMessages__MCN_304(t *testing.T) {
	row, history, rev := reversedTimeout(time.Now())
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{row.RRN: row}, history: history}, fakeReversals{rev: rev})
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/v1/transactions/"+row.RRN+"/journey", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `"code":"REVERSAL_CONFIRMED"`)
	require.Contains(t, body, `"mti":"0430"`)
	require.Contains(t, body, `"message":null`)
	require.Contains(t, body, `"latencyMs":null`) // a timeout has no issuer answer
	require.NotContains(t, body, "9704360000004417")
	requireMatchesSpec(t, req, rec)
}

func TestGetTransactions_fillsLatencyMs__MCN_304(t *testing.T) {
	now := time.Now()
	sent, responded := now, now.Add(162*time.Millisecond)
	row := store.TranLogRow{RRN: "a", Type: tranTypePurchase, Status: statusApproved, ResponseCode: "00", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName, CreatedAt: now, SentAt: &sent, RespondedAt: &responded}
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{page: []store.TranLogRow{row}, byRRN: map[string]store.TranLogRow{"a": row}}, fakeReversals{})

	for _, path := range []string{"/v1/transactions", "/v1/transactions/a"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		require.Contains(t, rec.Body.String(), `"latencyMs":162`, path)
	}
}

func requireMatchesSpec(t *testing.T, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	router, err := gorillamux.NewRouter(loadSpec(t))
	require.NoError(t, err)
	route, pathParams, err := router.FindRoute(req)
	require.NoError(t, err)
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route},
		Status:                 rec.Code,
		Header:                 rec.Header(),
	}
	input.SetBodyBytes(rec.Body.Bytes())
	require.NoError(t, openapi3filter.ValidateResponse(context.Background(), input))
}
