package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/store"
)

// TerminalLister is the port GET /v1/terminals needs; *store.TerminalRepository satisfies it.
type TerminalLister interface {
	List(ctx context.Context) ([]store.FixtureTerminal, error)
}

// terminal mirrors contracts/openapi.yaml's Terminal schema.
type terminal struct {
	TerminalID   string `json:"terminalId"`
	MerchantID   string `json:"merchantId"`
	MerchantName string `json:"merchantName"`
	MCC          string `json:"mcc"`
}

// MountTerminals registers GET /v1/terminals (NET-G13 / POS-G11).
func MountTerminals(r chi.Router, lister TerminalLister) {
	r.Get("/v1/terminals", func(w http.ResponseWriter, req *http.Request) {
		rows, err := lister.List(req.Context())
		if err != nil {
			problem(w, req, http.StatusInternalServerError, problemInternal, "could not read terminals")
			return
		}
		out := make([]terminal, 0, len(rows))
		for _, t := range rows {
			out = append(out, terminal(t))
		}
		writeJSONBody(w, http.StatusOK, out)
	})
}
