package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeTerminalLister struct{ terminals []store.FixtureTerminal }

func (f fakeTerminalLister) List(context.Context) ([]store.FixtureTerminal, error) {
	return f.terminals, nil
}

func TestGetTerminals_listsTerminalsWithMerchants__NET_G13(t *testing.T) {
	r := chi.NewRouter()
	MountTerminals(r, fakeTerminalLister{terminals: []store.FixtureTerminal{
		{TerminalID: "00000042", MerchantID: fixtureMID, MerchantName: "Cà phê Góc Phố", MCC: "5814"},
	}})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/terminals", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []map[string]string{{"terminalId": "00000042", "merchantId": fixtureMID, "merchantName": "Cà phê Góc Phố", "mcc": "5814"}}, got)
}
