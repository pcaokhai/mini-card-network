package api

import (
	"context"
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
		key, ok := idempotencyKey(w, req)
		if !ok {
			return
		}
		var body advtxn.PreAuthRequest
		if !decodeBody(w, req, &body, checkCardPresentWithAmount) {
			return
		}
		txn, err := svc.CreatePreAuth(req.Context(), body, key)
		if err != nil {
			transactionProblem(w, req, err, "pre-authorization-failed")
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCreateCompletion(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key, ok := idempotencyKey(w, req)
		if !ok {
			return
		}
		rrn, ok := pathRRN(w, req)
		if !ok {
			return
		}
		var body advtxn.CompletionRequest
		if !decodeBody(w, req, &body, func(c advtxn.CompletionRequest) fieldErrors { return checkAmount(c.Amount) }) {
			return
		}
		txn, err := svc.CreateCompletion(req.Context(), rrn, body, key)
		if err != nil {
			transactionProblem(w, req, err, "completion-failed")
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCreateRefund(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key, ok := idempotencyKey(w, req)
		if !ok {
			return
		}
		var body advtxn.RefundRequest
		if !decodeBody(w, req, &body, checkCardPresentWithAmount) {
			return
		}
		txn, err := svc.CreateRefund(req.Context(), body, key)
		if err != nil {
			transactionProblem(w, req, err, "refund-failed")
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCreateBalanceInquiry(svc AdvancedTransactor) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key, ok := idempotencyKey(w, req)
		if !ok {
			return
		}
		var body advtxn.BalanceInquiryRequest
		if !decodeBody(w, req, &body, checkBalanceInquiry) {
			return
		}
		txn, err := svc.CreateBalanceInquiry(req.Context(), body, key)
		if err != nil {
			transactionProblem(w, req, err, "balance-inquiry-failed")
			return
		}
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func checkCardPresentWithAmount(r advtxn.PreAuthRequest) fieldErrors {
	var errs fieldErrors
	checkCardPresent(&errs, r.TerminalID, r.CardToken, r.EntryMode)
	checkMoney(&errs, r.Amount)
	return errs
}

func checkBalanceInquiry(r advtxn.BalanceInquiryRequest) fieldErrors {
	var errs fieldErrors
	checkCardPresent(&errs, r.TerminalID, r.CardToken, r.EntryMode)
	return errs
}
