package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/advtxn"
)

// AdvancedTransactor is the port advtxn.go needs; *advtxn.Service satisfies it.
type AdvancedTransactor interface {
	CreatePreAuth(ctx context.Context, req advtxn.PreAuthRequest, idempotencyKey string) (advtxn.Transaction, error)
	CreateCompletion(ctx context.Context, rrn string, req advtxn.CompletionRequest, idempotencyKey string) (advtxn.Transaction, error)
	CreateRefund(ctx context.Context, req advtxn.RefundRequest, idempotencyKey string) (advtxn.Transaction, error)
	CreateBalanceInquiry(ctx context.Context, req advtxn.BalanceInquiryRequest, idempotencyKey string) (advtxn.Transaction, error)
}

// MountAdvancedTransactions registers the pre-auth/completion/refund/balance-inquiry routes
// (contracts/openapi.yaml, tag "transactions").
func MountAdvancedTransactions(r chi.Router, svc AdvancedTransactor) {
	r.Post("/v1/transactions/pre-authorizations", handleCreatePreAuth(svc))
	r.Post("/v1/transactions/{rrn}/completions", handleCreateCompletion(svc))
	r.Post("/v1/transactions/refunds", handleCreateRefund(svc))
	r.Post("/v1/transactions/balance-inquiries", handleCreateBalanceInquiry(svc))
}

func handleCreatePreAuth(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idempotencyKey := req.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body advtxn.PreAuthRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		txn, err := svc.CreatePreAuth(req.Context(), body, idempotencyKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "pre-authorization-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCreateCompletion(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idempotencyKey := req.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body advtxn.CompletionRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		rrn := chi.URLParam(req, "rrn")
		txn, err := svc.CreateCompletion(req.Context(), rrn, body, idempotencyKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "completion-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCreateRefund(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idempotencyKey := req.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body advtxn.RefundRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		txn, err := svc.CreateRefund(req.Context(), body, idempotencyKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "refund-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCreateBalanceInquiry(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idempotencyKey := req.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body advtxn.BalanceInquiryRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		txn, err := svc.CreateBalanceInquiry(req.Context(), body, idempotencyKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "balance-inquiry-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}
