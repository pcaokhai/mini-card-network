package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/advtxn"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	testIdemKey       = "11111111-1111-1111-1111-111111111111"
	validPurchaseBody = `{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":10000,"currency":"704"}}`
)

func postJSON(r http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Idempotency-Key", testIdemKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestPostPurchase_unknownTerminalIs422__MCN_002(t *testing.T) {
	r := chi.NewRouter()
	MountPurchases(r, &fakePurchaseService{err: fmt.Errorf("resolve merchant: %w", store.ErrUnknownTerminal)})

	rec := postJSON(r, "/v1/transactions/purchases", validPurchaseBody)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "unknown-terminal")
}

func TestPostPurchase_otherFailuresStay500__MCN_002(t *testing.T) {
	r := chi.NewRouter()
	MountPurchases(r, &fakePurchaseService{err: errors.New("db down")})

	rec := postJSON(r, "/v1/transactions/purchases", validPurchaseBody)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), `"type":"https://mcn.local/problems/internal"`, "an unexpected failure is the catalogue's internal problem (P-1)")
}

func TestPostCompletion_unknownPreAuthIs404__MCN_002(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{err: fmt.Errorf("look up pre-authorization: %w", store.ErrNotFound)})

	rec := postJSON(r, "/v1/transactions/000000000000/completions", `{"amount":{"amount":1,"currency":"704"}}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), `"type":"https://mcn.local/problems/not-found"`)
}

func TestPostRefund_unknownTerminalIs422__MCN_002(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{err: store.ErrUnknownTerminal, result: advtxn.Transaction{}})

	rec := postJSON(r, "/v1/transactions/refunds", validPurchaseBody)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestPostCancellation_notReversibleIs409__MCN_401(t *testing.T) {
	r := chi.NewRouter()
	MountPurchases(r, &fakePurchaseService{cancelErr: fmt.Errorf("cancel: %w", store.ErrNotReversible)})

	rec := postJSON(r, "/v1/transactions/626514000001/cancellations", `{}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "not-reversible")
}
