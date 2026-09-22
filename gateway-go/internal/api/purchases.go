package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/purchase"
)

// PurchaseCreator is the port purchases.go needs; *purchase.Service satisfies it.
type PurchaseCreator interface {
	CreatePurchase(ctx context.Context, req purchase.PurchaseRequest, idempotencyKey string) (purchase.Transaction, error)
}

// MountPurchases registers the purchase route (contracts/openapi.yaml, tag "transactions").
func MountPurchases(r chi.Router, svc PurchaseCreator) {
	r.Post("/v1/transactions/purchases", handleCreatePurchase(svc))
}

func handleCreatePurchase(svc PurchaseCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idempotencyKey := req.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body purchase.PurchaseRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		txn, err := svc.CreatePurchase(req.Context(), body, idempotencyKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "purchase-failed", err.Error())
			return
		}
		// Declines are not HTTP errors (contracts/openapi.yaml): every outcome returns 201.
		writeJSONBody(w, http.StatusCreated, txn)
	}
}
