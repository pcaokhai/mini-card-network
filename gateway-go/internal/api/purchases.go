package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/purchase"
)

// PurchaseCreator is the port purchases.go needs; *purchase.Service satisfies it.
type PurchaseCreator interface {
	CreatePurchase(ctx context.Context, req purchase.PurchaseRequest, idempotencyKey string) (purchase.Transaction, error)
	CancelPurchase(ctx context.Context, rrn string, idempotencyKey string) (purchase.Transaction, error)
}

// MountPurchases registers the purchase routes (contracts/openapi.yaml, tag "transactions").
func MountPurchases(r chi.Router, svc PurchaseCreator) {
	r.Post("/v1/transactions/purchases", handleCreatePurchase(svc))
	r.Post("/v1/transactions/{rrn}/cancellations", handleCancelPurchase(svc))
}

func handleCreatePurchase(svc PurchaseCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key, ok := idempotencyKey(w, req)
		if !ok {
			return
		}
		var body purchase.PurchaseRequest
		if !decodeBody(w, req, &body, checkPurchase) {
			return
		}
		txn, err := svc.CreatePurchase(req.Context(), body, key)
		if err != nil {
			transactionProblem(w, err, "purchase-failed")
			return
		}
		// Declines are not HTTP errors (contracts/openapi.yaml): every outcome returns 201.
		writeJSONBody(w, http.StatusCreated, txn)
	}
}

func handleCancelPurchase(svc PurchaseCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key, ok := idempotencyKey(w, req)
		if !ok {
			return
		}
		rrn, ok := pathRRN(w, req)
		if !ok {
			return
		}
		txn, err := svc.CancelPurchase(req.Context(), rrn, key)
		if err != nil {
			transactionProblem(w, err, "cancellation-failed")
			return
		}
		writeJSONBody(w, http.StatusAccepted, txn)
	}
}

func checkPurchase(r purchase.PurchaseRequest) fieldErrors {
	var errs fieldErrors
	checkCardPresent(&errs, r.TerminalID, r.CardToken, r.EntryMode)
	checkMoney(&errs, r.Amount)
	return errs
}
