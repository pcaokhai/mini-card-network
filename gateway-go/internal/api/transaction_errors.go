package api

import (
	"errors"
	"net/http"

	"github.com/mcn/gateway-go/internal/advtxn"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

// transactionProblem is the one place transaction-creating handlers turn a service error into a
// problem response (root CLAUDE.md §6.8). fallbackType names the failure when the error is not a
// known domain error.
func transactionProblem(w http.ResponseWriter, err error, fallbackType string) {
	switch {
	case errors.Is(err, store.ErrUnknownTerminal):
		problem(w, http.StatusUnprocessableEntity, "unknown-terminal", err.Error())
	case errors.Is(err, purchase.ErrUnknownCardToken):
		problem(w, http.StatusUnprocessableEntity, "unknown-card-token", "cardToken is not a simulator card")
	case errors.Is(err, store.ErrIdempotencyKeyMismatch):
		problem(w, http.StatusUnprocessableEntity, "idempotency-key-mismatch", "Idempotency-Key was used for a different request")
	case errors.Is(err, store.ErrIdempotencyInProgress):
		problem(w, http.StatusConflict, "conflict", "A request with this Idempotency-Key is still in progress")
	case errors.Is(err, advtxn.ErrNotCompletable):
		problem(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, store.ErrNotReversible):
		problem(w, http.StatusConflict, "not-reversible", err.Error())
	case errors.Is(err, store.ErrNotFound):
		problem(w, http.StatusNotFound, "not-found", err.Error())
	default:
		problem(w, http.StatusInternalServerError, fallbackType, err.Error())
	}
}
