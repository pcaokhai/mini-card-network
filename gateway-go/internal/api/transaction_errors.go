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
func transactionProblem(w http.ResponseWriter, req *http.Request, err error, fallbackType string) {
	switch {
	case errors.Is(err, store.ErrUnknownTerminal):
		problem(w, req, http.StatusUnprocessableEntity, "unknown-terminal", err.Error())
	case errors.Is(err, purchase.ErrUnknownCardToken):
		problem(w, req, http.StatusUnprocessableEntity, "unknown-card-token", "cardToken is not a simulator card")
	case errors.Is(err, store.ErrIdempotencyKeyMismatch):
		problem(w, req, http.StatusUnprocessableEntity, "idempotency-key-mismatch", "Idempotency-Key was used for a different request")
	case errors.Is(err, store.ErrIdempotencyInProgress), errors.Is(err, store.ErrReservationLost):
		problem(w, req, http.StatusConflict, problemConflict, "A request with this Idempotency-Key is still in progress")
	case errors.Is(err, advtxn.ErrNotCompletable), errors.Is(err, advtxn.ErrExceedsHold):
		problem(w, req, http.StatusConflict, problemConflict, err.Error())
	case errors.Is(err, store.ErrNotReversible):
		problem(w, req, http.StatusConflict, "not-reversible", err.Error())
	case errors.Is(err, store.ErrNotFound):
		problem(w, req, http.StatusNotFound, "not-found", err.Error())
	default:
		internalProblem(w, req, fallbackType, err)
	}
}
