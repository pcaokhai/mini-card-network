package api

import (
	"errors"
	"net/http"

	"github.com/mcn/gateway-go/internal/store"
)

// transactionProblem is the one place transaction-creating handlers turn a service error into a
// problem response (root CLAUDE.md §6.8). fallbackType names the failure when the error is not a
// known domain error.
func transactionProblem(w http.ResponseWriter, err error, fallbackType string) {
	switch {
	case errors.Is(err, store.ErrUnknownTerminal):
		problem(w, http.StatusUnprocessableEntity, "unknown-terminal", err.Error())
	case errors.Is(err, store.ErrNotFound):
		problem(w, http.StatusNotFound, "unknown-transaction", err.Error())
	default:
		problem(w, http.StatusInternalServerError, fallbackType, err.Error())
	}
}
