package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

type fakeLinkReader struct{ link store.Link }

func (f *fakeLinkReader) Get(context.Context, string) (store.Link, error) { return f.link, nil }
func (f *fakeLinkReader) ListEvents(context.Context, int) ([]store.NetworkEvent, error) {
	return []store.NetworkEvent{{ID: 1, OccurredAt: time.Now(), Severity: "INFO", EasyText: "Link is healthy", TechnicalText: "issuer SIGNED_ON"}}, nil
}

type fakeTrigger struct {
	echoResult isonet.EchoResult
	echoErr    error
	signOnErr  error
	signOffErr error
}

func (f *fakeTrigger) TriggerEcho(context.Context) (isonet.EchoResult, error) {
	return f.echoResult, f.echoErr
}
func (f *fakeTrigger) TriggerSignOn(context.Context) error  { return f.signOnErr }
func (f *fakeTrigger) TriggerSignOff(context.Context) error { return f.signOffErr }

type fakeSafReader struct {
	items     []store.SafItemRow
	deadCount int
}

func (f *fakeSafReader) ListItems(context.Context) ([]store.SafItemRow, int, error) {
	return f.items, f.deadCount, nil
}

func TestGetSaf_returnsDepthAndDeadCount__MCN_407_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, nil, &fakeSafReader{
		items: []store.SafItemRow{
			{ID: 1, MTI: "0420", RRN: "626514000999", AmountMinor: 10000, Currency: "704", Attempts: 1, Status: "PENDING", NextRetryAt: time.Now()},
			{ID: 2, MTI: "0420", RRN: "626514001111", AmountMinor: 5000, Currency: "704", Attempts: 5, Status: "DEAD", NextRetryAt: time.Now()},
		},
		deadCount: 1,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/network/saf", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"depth":2`)
	require.Contains(t, rec.Body.String(), `"deadCount":1`)
	require.Contains(t, rec.Body.String(), `"rrn":"626514000999"`)
}

func TestGetLinks_returnsCurrentStatus__MCN_204_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{link: store.Link{Endpoint: "issuer", Status: "SIGNED_ON"}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/network/links", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"SIGNED_ON"`)
	require.Contains(t, rec.Body.String(), `"linkId":"issuer"`)
}

func TestGetNetworkEvents_returnsRecentEventsEnvelope__MCN_204_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/network/events", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"items"`)
	require.Contains(t, rec.Body.String(), "Link is healthy")
}

func TestPostEcho_returnsEchoResult__MCN_204_AC3(t *testing.T) {
	latency := 12
	rc := "00"
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, &fakeTrigger{echoResult: isonet.EchoResult{OK: true, LatencyMs: &latency, ResponseCode: &rc}})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/issuer/echo", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok":true`)
	require.Contains(t, rec.Body.String(), `"latencyMs":12`)
}

func TestPostEcho_downLinkReturnsOkFalseNotError__MCN_204_AC3(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, &fakeTrigger{echoResult: isonet.EchoResult{OK: false}})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/issuer/echo", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok":false`)
}

func TestPostEcho_unknownLinkIdReturns404__MCN_204_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, &fakeTrigger{})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/switch/echo", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPostSignOn_returnsUpdatedLink__MCN_204_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{link: store.Link{Endpoint: "issuer", Status: "SIGNED_ON"}}, &fakeTrigger{})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/issuer/sign-on", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"SIGNED_ON"`)
}

func TestPostSignOn_conflictWhenNotConnected__MCN_204_AC3(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, &fakeTrigger{signOnErr: errors.New("link is not signed on")})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/issuer/sign-on", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}
