package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/advtxn"
)

type fakeAdvTxn struct{ result advtxn.Transaction }

func (f *fakeAdvTxn) CreatePreAuth(context.Context, advtxn.PreAuthRequest, string) (advtxn.Transaction, error) {
	return f.result, nil
}
func (f *fakeAdvTxn) CreateCompletion(context.Context, string, advtxn.CompletionRequest, string) (advtxn.Transaction, error) {
	return f.result, nil
}
func (f *fakeAdvTxn) CreateRefund(context.Context, advtxn.RefundRequest, string) (advtxn.Transaction, error) {
	return f.result, nil
}
func (f *fakeAdvTxn) CreateBalanceInquiry(context.Context, advtxn.BalanceInquiryRequest, string) (advtxn.Transaction, error) {
	return f.result, nil
}

func TestPostPreAuthorizations_returns201__MCN_603_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{result: advtxn.Transaction{Type: "PREAUTH"}})

	body := []byte(`{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":1000,"currency":"704"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/pre-authorizations", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "11111111-1111-1111-1111-111111111111")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestPostCompletions_returns201__MCN_603_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{result: advtxn.Transaction{Type: "COMPLETION"}})

	body := []byte(`{"amount":{"amount":1000,"currency":"704"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/123456789012/completions", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestPostRefunds_returns201__MCN_603_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{result: advtxn.Transaction{Type: "REFUND"}})

	body := []byte(`{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":1000,"currency":"704"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/refunds", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestPostBalanceInquiries_requiresIdempotencyKey__MCN_603_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{})

	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/balance-inquiries", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPostBalanceInquiries_returns201__MCN_603_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountAdvancedTransactions(r, &fakeAdvTxn{result: advtxn.Transaction{Type: "BALANCE"}})

	body := []byte(`{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/balance-inquiries", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}
