package api

import (
	"net/http"

	"github.com/google/uuid"
)

// docs/04 §3 catalogue slugs used by the network and key routes.
// ponytail: chaos.go (#118) declares its own copies; fold both into one set with the shared
// URI problem writer (NET-G17 / SEC-G7).
const (
	problemValidation          = "validation-error"
	problemNotFound            = "not-found"
	problemInternal            = "internal"
	problemIdempotencyKey      = "insufficient-idempotency-key"
	problemIdempotencyMismatch = "idempotency-key-mismatch"
)

// requireUUIDKey enforces docs/04 §2: a state-changing call carries a UUID Idempotency-Key.
func requireUUIDKey(w http.ResponseWriter, req *http.Request) (string, bool) {
	key := req.Header.Get("Idempotency-Key")
	if uuid.Validate(key) != nil {
		problem(w, http.StatusBadRequest, problemIdempotencyKey, "Idempotency-Key header must be a UUID")
		return "", false
	}
	return key, true
}
