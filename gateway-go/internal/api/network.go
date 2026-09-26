package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

// LinkReader is the read-side port network.go needs; store.LinkRepository satisfies it.
type LinkReader interface {
	Get(ctx context.Context, endpoint string) (store.Link, error)
	ListEvents(ctx context.Context, limit int, cursor string) ([]store.NetworkEvent, string, error)
}

// LinkTrigger is the manual-action port; isonet.Supervisor satisfies it.
type LinkTrigger interface {
	TriggerEcho(ctx context.Context) (isonet.EchoResult, error)
	TriggerSignOn(ctx context.Context) error
	TriggerSignOff(ctx context.Context) error
	LinkMetrics() (p99LatencyMs *int, inFlight int)
}

// SafReader is the port GET /v1/network/saf needs; *store.SafRepository satisfies it.
type SafReader interface {
	ListItems(ctx context.Context) (store.SafSnapshot, error)
}

const issuerLinkID = "issuer" // v1 has exactly one link; the switch (Sprint 10) adds more.

const (
	defaultEventsLimit = 50 // components.parameters.Limit
	maxEventsLimit     = 200
)

// MountNetwork registers the network operations routes (contracts/openapi.yaml, tag "network").
// safReader is optional (nil/omitted disables GET /v1/network/saf) so existing callers that
// don't need SAF status keep working unchanged.
// trigger may be nil in read-only tests; the links then carry the stored metrics only.
func MountNetwork(r chi.Router, reader LinkReader, trigger LinkTrigger, safReader ...SafReader) {
	replays := newReplayStore()
	r.Get("/v1/network/links", handleListLinks(reader, trigger))
	r.Get("/v1/network/events", handleListNetworkEvents(reader))
	r.Post("/v1/network/links/{linkId}/echo", handleEchoLink(trigger))
	r.Post("/v1/network/links/{linkId}/sign-on", handleLinkTransition(reader, trigger, replays, "sign-on", func(ctx context.Context) error { return trigger.TriggerSignOn(ctx) }))
	r.Post("/v1/network/links/{linkId}/sign-off", handleLinkTransition(reader, trigger, replays, "sign-off", func(ctx context.Context) error { return trigger.TriggerSignOff(ctx) }))
	if len(safReader) > 0 && safReader[0] != nil {
		r.Get("/v1/network/saf", handleGetSaf(safReader[0]))
	}
}

func handleGetSaf(reader SafReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		snap, err := reader.ListItems(req.Context())
		if err != nil {
			problem(w, req, http.StatusInternalServerError, problemInternal, "could not read the SAF queue")
			return
		}
		writeJSONBody(w, http.StatusOK, struct {
			Depth     int           `json:"depth"`
			DeadCount int           `json:"deadCount"`
			Items     []safItemBody `json:"items"`
		}{Depth: snap.Depth, DeadCount: snap.DeadCount, Items: toSafItemBodies(snap.Items)})
	}
}

type safItemBody struct {
	ID          string  `json:"id"`
	MTI         string  `json:"mti"`
	RRN         string  `json:"rrn"`
	Amount      money   `json:"amount"`
	Attempts    int     `json:"attempts"`
	Status      string  `json:"status"`
	NextRetryAt string  `json:"nextRetryAt"`
	LastError   *string `json:"lastError"`
}

type money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func toSafItemBodies(items []store.SafItemRow) []safItemBody {
	bodies := make([]safItemBody, 0, len(items))
	for _, it := range items {
		var lastError *string
		if it.LastError != "" {
			lastError = &it.LastError
		}
		bodies = append(bodies, safItemBody{
			ID:          strconv.FormatInt(it.ID, 10),
			MTI:         it.MTI,
			RRN:         it.RRN,
			Amount:      money{Amount: it.AmountMinor, Currency: it.Currency},
			Attempts:    it.Attempts,
			Status:      it.Status,
			NextRetryAt: it.NextRetryAt.Format(time.RFC3339),
			LastError:   lastError,
		})
	}
	return bodies
}

func handleListLinks(reader LinkReader, trigger LinkTrigger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		link, ok := readLink(w, req, reader, trigger)
		if !ok {
			return
		}
		writeJSONBody(w, http.StatusOK, []store.Link{link})
	}
}

// readLink reads the stored link and overlays the supervisor's live p99 and in-flight count
// (NET-G16).
func readLink(w http.ResponseWriter, req *http.Request, reader LinkReader, trigger LinkTrigger) (store.Link, bool) {
	link, err := reader.Get(req.Context(), issuerLinkID)
	if err != nil {
		problem(w, req, http.StatusInternalServerError, problemInternal, "could not read the link state")
		return store.Link{}, false
	}
	if trigger != nil {
		link.P99LatencyMs, link.InFlight = trigger.LinkMetrics()
	}
	return link, true
}

func handleListNetworkEvents(reader LinkReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		limit, ok := pageLimit(w, req)
		if !ok {
			return
		}
		events, next, err := reader.ListEvents(req.Context(), limit, req.URL.Query().Get("cursor"))
		if errors.Is(err, store.ErrInvalidCursor) {
			problem(w, req, http.StatusBadRequest, problemValidation, "cursor is not one this endpoint issued")
			return
		}
		if err != nil {
			problem(w, req, http.StatusInternalServerError, problemInternal, "could not read network events")
			return
		}
		var nextCursor *string
		if next != "" {
			nextCursor = &next
		}
		if events == nil {
			events = []store.NetworkEvent{}
		}
		writeJSONBody(w, http.StatusOK, struct {
			Items      []store.NetworkEvent `json:"items"`
			NextCursor *string              `json:"nextCursor"`
		}{Items: events, NextCursor: nextCursor})
	}
}

// pageLimit reads ?limit= (docs/04 §2: default 50, max 200).
func pageLimit(w http.ResponseWriter, req *http.Request) (int, bool) {
	raw := req.URL.Query().Get("limit")
	if raw == "" {
		return defaultEventsLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxEventsLimit {
		problem(w, req, http.StatusBadRequest, problemValidation, "limit must be an integer from 1 to 200")
		return 0, false
	}
	return n, true
}

// handleEchoLink requires a UUID key but does not replay it: an echo only reads link health, so
// repeating it is harmless, and a replayed stale result would be misleading (NET-G14 Ruling).
func handleEchoLink(trigger LinkTrigger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !validLinkID(w, req) {
			return
		}
		if _, ok := requireUUIDKey(w, req); !ok {
			return
		}
		result, err := trigger.TriggerEcho(req.Context())
		if err != nil {
			problem(w, req, http.StatusInternalServerError, problemInternal, "echo could not be sent")
			return
		}
		writeJSONBody(w, http.StatusOK, struct {
			OK           bool    `json:"ok"`
			LatencyMs    *int    `json:"latencyMs"`
			ResponseCode *string `json:"responseCode"`
		}{result.OK, result.LatencyMs, result.ResponseCode})
	}
}

// handleLinkTransition runs a sign-on or sign-off once per Idempotency-Key and replays the
// resulting Link for a retry of the same action (NET-G14).
func handleLinkTransition(reader LinkReader, trigger LinkTrigger, replays *replayStore, action string, call func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !validLinkID(w, req) {
			return
		}
		key, ok := requireUUIDKey(w, req)
		if !ok {
			return
		}
		replays.serve(req, w, key, action, func(ctx context.Context) (int, any, bool) {
			if err := call(ctx); err != nil {
				problem(w, req, http.StatusConflict, "link-not-ready", err.Error())
				return 0, nil, false
			}
			link, ok := readLink(w, req, reader, trigger)
			return http.StatusOK, link, ok
		})
	}
}

func validLinkID(w http.ResponseWriter, req *http.Request) bool {
	if chi.URLParam(req, "linkId") != issuerLinkID {
		problem(w, req, http.StatusNotFound, problemNotFound, "no such link")
		return false
	}
	return true
}
