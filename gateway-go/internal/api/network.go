package api

import (
	"context"
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
	ListEvents(ctx context.Context, limit int) ([]store.NetworkEvent, error)
}

// LinkTrigger is the manual-action port; isonet.Supervisor satisfies it.
type LinkTrigger interface {
	TriggerEcho(ctx context.Context) (isonet.EchoResult, error)
	TriggerSignOn(ctx context.Context) error
	TriggerSignOff(ctx context.Context) error
}

// SafReader is the port GET /v1/network/saf needs; *store.SafRepository satisfies it.
type SafReader interface {
	ListItems(ctx context.Context) ([]store.SafItemRow, int, error)
}

const issuerLinkID = "issuer" // v1 has exactly one link; the switch (Sprint 10) adds more.

const defaultEventsLimit = 50

// MountNetwork registers the network operations routes (contracts/openapi.yaml, tag "network").
// safReader is optional (nil/omitted disables GET /v1/network/saf) so existing callers that
// don't need SAF status keep working unchanged.
func MountNetwork(r chi.Router, reader LinkReader, trigger LinkTrigger, safReader ...SafReader) {
	r.Get("/v1/network/links", handleListLinks(reader))
	r.Get("/v1/network/events", handleListNetworkEvents(reader))
	r.Post("/v1/network/links/{linkId}/echo", handleEchoLink(trigger))
	r.Post("/v1/network/links/{linkId}/sign-on", handleSignOnLink(reader, trigger))
	r.Post("/v1/network/links/{linkId}/sign-off", handleSignOffLink(reader, trigger))
	if len(safReader) > 0 && safReader[0] != nil {
		r.Get("/v1/network/saf", handleGetSaf(safReader[0]))
	}
}

func handleGetSaf(reader SafReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		items, deadCount, err := reader.ListItems(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, "saf-read-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, struct {
			Depth     int           `json:"depth"`
			DeadCount int           `json:"deadCount"`
			Items     []safItemBody `json:"items"`
		}{Depth: len(items), DeadCount: deadCount, Items: toSafItemBodies(items)})
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

func handleListLinks(reader LinkReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		link, err := reader.Get(req.Context(), issuerLinkID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "link-read-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, []store.Link{link})
	}
}

func handleListNetworkEvents(reader LinkReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		events, err := reader.ListEvents(req.Context(), defaultEventsLimit)
		if err != nil {
			problem(w, http.StatusInternalServerError, "events-read-failed", err.Error())
			return
		}
		// ponytail: no keyset pagination yet (nextCursor always null); add cursor scanning
		// once event volume outgrows a single page of `limit`.
		writeJSONBody(w, http.StatusOK, struct {
			Items      []store.NetworkEvent `json:"items"`
			NextCursor *string              `json:"nextCursor"`
		}{Items: events, NextCursor: nil})
	}
}

func handleEchoLink(trigger LinkTrigger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !validLinkID(w, req) {
			return
		}
		result, err := trigger.TriggerEcho(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, "echo-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, struct {
			OK           bool    `json:"ok"`
			LatencyMs    *int    `json:"latencyMs"`
			ResponseCode *string `json:"responseCode"`
		}{result.OK, result.LatencyMs, result.ResponseCode})
	}
}

func handleSignOnLink(reader LinkReader, trigger LinkTrigger) http.HandlerFunc {
	return handleLinkTransition(reader, func(ctx context.Context) error { return trigger.TriggerSignOn(ctx) })
}

func handleSignOffLink(reader LinkReader, trigger LinkTrigger) http.HandlerFunc {
	return handleLinkTransition(reader, func(ctx context.Context) error { return trigger.TriggerSignOff(ctx) })
}

func handleLinkTransition(reader LinkReader, call func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !validLinkID(w, req) {
			return
		}
		if err := call(req.Context()); err != nil {
			problem(w, http.StatusConflict, "link-not-ready", err.Error())
			return
		}
		link, err := reader.Get(req.Context(), issuerLinkID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "link-read-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, link)
	}
}

func validLinkID(w http.ResponseWriter, req *http.Request) bool {
	if chi.URLParam(req, "linkId") != issuerLinkID {
		problem(w, http.StatusNotFound, "unknown-link", "no such link")
		return false
	}
	return true
}
