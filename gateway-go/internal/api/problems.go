package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/mcn/gateway-go/internal/obs"
)

// docs/04 §3 catalogue slugs.
const (
	problemValidation          = "validation-error"
	problemNotFound            = "not-found"
	problemInternal            = "internal"
	problemIdempotencyKey      = "insufficient-idempotency-key"
	problemIdempotencyMismatch = "idempotency-key-mismatch"
	problemConflict            = "conflict"
)

// fieldBody names the request body itself in errors[] when it can't be decoded at all.
const fieldBody = "body"

// problemTypeBase prefixes every problem type (docs/04 §3).
const problemTypeBase = "https://mcn.local/problems/"

// problemTitles are the catalogue's human-readable titles; a slug without one uses the slug.
var problemTitles = map[string]string{
	problemValidation:          "The request is invalid",
	problemIdempotencyKey:      "Idempotency-Key must be a UUID",
	problemNotFound:            "Not found",
	"precondition-failed":      "The resource changed",
	problemConflict:            "The state doesn't allow this",
	"open-breaks":              "Reconciliation breaks are open",
	"not-reversible":           "The transaction can't be reversed",
	"link-not-ready":           "The issuer link isn't connected",
	"unknown-terminal":         "Unknown terminal",
	"unknown-card-token":       "Unknown card token",
	problemIdempotencyMismatch: "Idempotency-Key was used for a different request",
	"scenario-unavailable":     "This scenario can't run on this stack",
	problemInternal:            "Internal error",
}

// problemBody is docs/04 §3's problem+json shape.
type problemBody struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance"`
	TraceID  string       `json:"traceId,omitempty"`
	Errors   []fieldError `json:"errors,omitempty"`
}

// problem is the gateway's one problem writer (root CLAUDE.md §6.8): a catalogue slug as the full
// URI type, the request path as instance and the request's W3C trace id (P-1).
func problem(w http.ResponseWriter, req *http.Request, status int, slug, detail string) {
	writeProblem(w, req, problemBody{Status: status, Type: slug, Detail: detail})
}

func writeProblem(w http.ResponseWriter, req *http.Request, body problemBody) {
	slug := body.Type
	body.Type = problemTypeBase + slug
	body.Title = problemTitles[slug]
	if body.Title == "" {
		body.Title = slug
	}
	body.Instance = req.URL.Path
	body.TraceID = obs.TraceID(req.Context())
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(body.Status)
	_ = json.NewEncoder(w).Encode(body)
}

// internalProblem answers an unexpected error with a generic 500: the cause goes to the log
// under the request's trace id, never to the client (docs/04 §3). op names what failed.
func internalProblem(w http.ResponseWriter, req *http.Request, op string, err error) {
	slog.ErrorContext(req.Context(), "request failed", "op", op, "error", err.Error())
	problem(w, req, http.StatusInternalServerError, problemInternal, "The gateway could not complete the request; its log has the cause under this request's trace id")
}

// requireUUIDKey enforces docs/04 §2: a state-changing call carries a UUID Idempotency-Key.
func requireUUIDKey(w http.ResponseWriter, req *http.Request) (string, bool) {
	key := req.Header.Get("Idempotency-Key")
	if uuid.Validate(key) != nil {
		problem(w, req, http.StatusBadRequest, problemIdempotencyKey, "Idempotency-Key header must be a UUID")
		return "", false
	}
	return key, true
}
