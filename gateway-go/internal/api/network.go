package api

import (
	"context"
	"net/http"

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

const issuerLinkID = "issuer" // v1 has exactly one link; the switch (Sprint 10) adds more.

const defaultEventsLimit = 50

// MountNetwork registers the network operations routes (contracts/openapi.yaml, tag "network").
func MountNetwork(r chi.Router, reader LinkReader, trigger LinkTrigger) {
	r.Get("/v1/network/links", handleListLinks(reader))
	r.Get("/v1/network/events", handleListNetworkEvents(reader))
	r.Post("/v1/network/links/{linkId}/echo", handleEchoLink(trigger))
	r.Post("/v1/network/links/{linkId}/sign-on", handleSignOnLink(reader, trigger))
	r.Post("/v1/network/links/{linkId}/sign-off", handleSignOffLink(reader, trigger))
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
