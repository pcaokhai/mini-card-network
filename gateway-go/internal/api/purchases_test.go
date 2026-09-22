package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/purchase"
)

type fakePurchaseService struct {
	result purchase.Transaction
	err    error
}

func (f *fakePurchaseService) CreatePurchase(context.Context, purchase.PurchaseRequest, string) (purchase.Transaction, error) {
	return f.result, f.err
}

func TestPostPurchase_requiresIdempotencyKey__MCN_303_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountPurchases(r, &fakePurchaseService{})

	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/purchases", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPostPurchase_returns201WithTransaction__MCN_303_AC2(t *testing.T) {
	r := chi.NewRouter()
	svc := &fakePurchaseService{result: purchase.Transaction{RRN: "x", Status: "APPROVED"}}
	MountPurchases(r, svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/transactions/purchases", bytes.NewBufferString(`{"terminalId":"00000042","cardToken":"tok_normal","entryMode":"CHIP_PIN","amount":{"amount":10000,"currency":"704"}}`))
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"APPROVED"`)
}
