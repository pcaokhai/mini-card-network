package api

import (
	"errors"
	"log/slog"
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
		problem(w, http.StatusUnprocessableEntity, "unknown-terminal", err.Error())
	case errors.Is(err, purchase.ErrUnknownCardToken):
		problem(w, http.StatusUnprocessableEntity, "unknown-card-token", "cardToken is not a simulator card")
	case errors.Is(err, store.ErrIdempotencyKeyMismatch):
		problem(w, http.StatusUnprocessableEntity, "idempotency-key-mismatch", "Idempotency-Key was used for a different request")
	case errors.Is(err, store.ErrIdempotencyInProgress), errors.Is(err, store.ErrReservationLost):
		problem(w, http.StatusConflict, "conflict", "A request with this Idempotency-Key is still in progress")
	case errors.Is(err, advtxn.ErrNotCompletable), errors.Is(err, advtxn.ErrExceedsHold):
		problem(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, store.ErrNotReversible):
		problem(w, http.StatusConflict, "not-reversible", err.Error())
	case errors.Is(err, store.ErrNotFound):
		problem(w, http.StatusNotFound, "not-found", err.Error())
	default:
		// The cause stays in the log, under the request's trace id; the client gets no driver or
		// parser text (docs/04 §3).
		slog.ErrorContext(req.Context(), "transaction request failed", "problem", fallbackType, "error", err.Error())
		problem(w, http.StatusInternalServerError, fallbackType, "The gateway could not complete the request; its log has the cause under this request's trace id")
	}
}
