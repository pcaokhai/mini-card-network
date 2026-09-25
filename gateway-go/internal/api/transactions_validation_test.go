package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/advtxn"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	pathPurchases  = "/v1/transactions/purchases"
	statusReversed = "REVERSED"
	fieldAmount    = "amount.amount"
)

type problemBody struct {
	Type   string `json:"type"`
	Errors []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) problemBody {
	t.Helper()
	var p problemBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p), rec.Body.String())
	return p
}

func transactionRouter() chi.Router {
	r := chi.NewRouter()
	MountPurchases(r, &fakePurchaseService{result: purchase.Transaction{RRN: "626514000001", Status: statusApproved}})
	MountAdvancedTransactions(r, &fakeAdvTxn{result: advtxn.Transaction{RRN: "626514000001", Type: "PREAUTH"}})
	return r
}

func TestPostTransactions_invalidBodyIsValidationError__POS_G2(t *testing.T) {
	cases := []struct {
		name, path, body, field string
	}{
		{"zero amount", pathPurchases, `{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":0,"currency":"704"}}`, fieldAmount},
		{"negative amount", "/v1/transactions/refunds", `{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":-5,"currency":"704"}}`, fieldAmount},
		{"alpha currency", "/v1/transactions/pre-authorizations", `{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":1,"currency":"VND"}}`, "amount.currency"},
		{"unknown entry mode", pathPurchases, `{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"SWIPE","amount":{"amount":1,"currency":"704"}}`, "entryMode"},
		{"short terminal", "/v1/transactions/balance-inquiries", `{"terminalId":"42","cardToken":"tok_normal","entryMode":"CHIP_PIN"}`, "terminalId"},
		{"missing card token", "/v1/transactions/balance-inquiries", `{"terminalId":"00000042","entryMode":"CHIP_PIN"}`, "cardToken"},
		{"completion amount", "/v1/transactions/626514000001/completions", `{"amount":{"amount":0,"currency":"704"}}`, fieldAmount},
		{"not JSON", pathPurchases, `{`, "body"},
		{"malformed original rrn", "/v1/transactions/bad!!/completions", `{"amount":{"amount":1,"currency":"704"}}`, "rrn"},
		{"malformed cancelled rrn", "/v1/transactions/12/cancellations", `{}`, "rrn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(transactionRouter(), tc.path, tc.body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			p := decodeProblem(t, rec)
			require.Equal(t, "validation-error", p.Type)
			require.NotEmpty(t, p.Errors)
			require.Equal(t, tc.field, p.Errors[0].Field)
		})
	}
}

func TestPostTransactions_idempotencyKeyMustBeAUUID__POS_G3(t *testing.T) {
	for name, key := range map[string]string{"missing": "", "not a UUID": "k1"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, pathPurchases, strings.NewReader(validPurchaseBody))
			if key != "" {
				req.Header.Set("Idempotency-Key", key)
			}
			rec := httptest.NewRecorder()
			transactionRouter().ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "insufficient-idempotency-key", decodeProblem(t, rec).Type)
		})
	}
}

func TestTransactionProblem_mapsDomainErrorsToTheCatalogue__POS_G1_G3_G13(t *testing.T) {
	cases := []struct {
		err    error
		status int
		slug   string
	}{
		{fmt.Errorf("resolve: %w", purchase.ErrUnknownCardToken), http.StatusUnprocessableEntity, "unknown-card-token"},
		{fmt.Errorf("reserve: %w", store.ErrIdempotencyKeyMismatch), http.StatusUnprocessableEntity, "idempotency-key-mismatch"},
		{fmt.Errorf("reserve: %w", store.ErrIdempotencyInProgress), http.StatusConflict, "conflict"},
		{fmt.Errorf("complete: %w", advtxn.ErrNotCompletable), http.StatusConflict, "conflict"},
		{fmt.Errorf("look up: %w", store.ErrNotFound), http.StatusNotFound, "not-found"},
	}
	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			r := chi.NewRouter()
			MountAdvancedTransactions(r, &fakeAdvTxn{err: tc.err})

			rec := postJSON(r, "/v1/transactions/626514000001/completions", `{"amount":{"amount":1,"currency":"704"}}`)

			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, tc.slug, decodeProblem(t, rec).Type)
		})
	}
}

func TestPostPurchase_responseMatchesTheContract__POS_G8(t *testing.T) {
	label := "Approved"
	latency := 42
	r := chi.NewRouter()
	MountPurchases(r, &fakePurchaseService{result: purchase.Transaction{
		RRN: "626514000001", STAN: "000001", Type: tranTypePurchase, Status: statusApproved, ResponseCode: "00", ResponseLabel: &label,
		Amount: purchase.Money{Amount: 10000, Currency: "704"}, MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName,
		LatencyMs: &latency, CreatedAt: time.Now().UTC(), AuthCode: "123456", BusinessDate: "2026-09-25", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	}})
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/v1/transactions/purchases", strings.NewReader(validPurchaseBody))
	req.Header.Set("Idempotency-Key", testIdemKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	for _, field := range []string{`"responseLabel":"Approved"`, `"latencyMs":42`, `"traceId":"4bf92f3577b34da6a3ce929d0e0e4736"`} {
		require.Contains(t, rec.Body.String(), field)
	}
	requireMatchesSpec(t, req, rec)
}

func TestGetTransactions_invalidQueryIsValidationError__JRN_G6(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=201", "limit=abc", "status=BOGUS", "rc=1", "last4=12a4", "reversalReason=OOPS", "from=yesterday"} {
		t.Run(query, func(t *testing.T) {
			r := chi.NewRouter()
			MountTransactionsQuery(r, &fakeTranLogReader{}, fakeReversals{})
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/transactions?"+query, nil))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "validation-error", decodeProblem(t, rec).Type)
		})
	}
}

func TestGetTransactions_badCursorIsValidationError__JRN_G6(t *testing.T) {
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{listErr: fmt.Errorf("%w: bad base64", store.ErrInvalidCursor)}, fakeReversals{})
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/transactions?cursor=zzz", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "validation-error", decodeProblem(t, rec).Type)
}

func TestGetTransaction_malformedRRNIsValidationError__JRN_G6(t *testing.T) {
	for _, path := range []string{"/v1/transactions/bad!!", "/v1/transactions/bad!!/journey"} {
		r := chi.NewRouter()
		MountTransactionsQuery(r, &fakeTranLogReader{}, fakeReversals{})
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		require.Equal(t, http.StatusBadRequest, rec.Code, path)
	}
}

func TestGetTransactions_filtersAndReportsTheReversalReason__JRN_G7(t *testing.T) {
	row := store.TranLogRow{RRN: "626514000003", Type: tranTypePurchase, Status: statusReversed, ResponseCode: "00", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName, CreatedAt: time.Now().UTC(), ReversalReasonCode: "68"}
	reader := &fakeTranLogReader{page: []store.TranLogRow{row}}
	r := chi.NewRouter()
	MountTransactionsQuery(r, reader, fakeReversals{})
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/v1/transactions?reversalReason=TIMEOUT&limit=200", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, store.ReversalTimeout, *reader.filter.ReversalReason)
	require.Equal(t, 200, reader.filter.Limit)
	require.Contains(t, rec.Body.String(), `"reversalReason":"TIMEOUT"`)
	requireMatchesSpec(t, req, rec)
}

func TestGetTransaction_reportsTheDetailFields__JRN_G3(t *testing.T) {
	approved := int64(4000)
	row := store.TranLogRow{
		RRN: "626514000004", Type: "COMPLETION", Status: statusApproved, ResponseCode: "10", Amount: 5000, Currency: "704",
		MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantName: testMerchantName, CreatedAt: time.Now().UTC(),
		ApprovedAmount: &approved, Balance: &store.Money{Amount: 42000, Currency: "704"}, OriginalRRN: "626514000002",
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	}
	r := chi.NewRouter()
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{row.RRN: row}}, fakeReversals{})
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/v1/transactions/"+row.RRN, nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, field := range []string{
		`"approvedAmount":{"amount":4000,"currency":"704"}`, `"balance":{"amount":42000,"currency":"704"}`,
		`"originalRrn":"626514000002"`, `"traceId":"4bf92f3577b34da6a3ce929d0e0e4736"`,
	} {
		require.Contains(t, body, field)
	}
	requireMatchesSpec(t, req, rec)
}
