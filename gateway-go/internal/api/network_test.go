package api

import (
	"context"
	"encoding/json"
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

type fakeLinkReader struct {
	link       store.Link
	nextCursor string
	cursorErr  error
	gotLimit   int
	gotCursor  string
}

func (f *fakeLinkReader) Get(context.Context, string) (store.Link, error) { return f.link, nil }
func (f *fakeLinkReader) ListEvents(_ context.Context, limit int, cursor string) ([]store.NetworkEvent, string, error) {
	f.gotLimit, f.gotCursor = limit, cursor
	if f.cursorErr != nil {
		return nil, "", f.cursorErr
	}
	return []store.NetworkEvent{{ID: 1, OccurredAt: time.Now(), Code: "LINK_UP", Severity: "INFO", EasyText: "Link is healthy", TechnicalText: "issuer SIGNED_ON"}}, f.nextCursor, nil
}

type fakeTrigger struct {
	echoResult isonet.EchoResult
	echoErr    error
	signOnErr  error
	signOffErr error
	signOns    int
	p99        *int
	inFlight   int
}

func (f *fakeTrigger) TriggerEcho(context.Context) (isonet.EchoResult, error) {
	return f.echoResult, f.echoErr
}
func (f *fakeTrigger) TriggerSignOn(context.Context) error {
	f.signOns++
	return f.signOnErr
}
func (f *fakeTrigger) LinkMetrics() (*int, int)             { return f.p99, f.inFlight }
func (f *fakeTrigger) TriggerSignOff(context.Context) error { return f.signOffErr }

type fakeSafReader struct{ snap store.SafSnapshot }

func (f *fakeSafReader) ListItems(context.Context) (store.SafSnapshot, error) { return f.snap, nil }

func TestGetSaf_returnsDepthAndDeadCount__MCN_407_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, nil, &fakeSafReader{snap: store.SafSnapshot{
		Items: []store.SafItemRow{
			{ID: 1, MTI: "0420", RRN: "626514000999", AmountMinor: 10000, Currency: "704", Attempts: 1, Status: "PENDING", NextRetryAt: time.Now()},
			{ID: 2, MTI: "0420", RRN: "626514001111", AmountMinor: 5000, Currency: "704", Attempts: 5, Status: "DEAD", NextRetryAt: time.Now()},
		},
		Depth:     1,
		DeadCount: 1,
	}})

	req := httptest.NewRequest(http.MethodGet, "/v1/network/saf", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"depth":1`) // PENDING + IN_FLIGHT only (NET-G7)
	require.Contains(t, rec.Body.String(), `"deadCount":1`)
	require.Contains(t, rec.Body.String(), `"rrn":"626514000999"`)
}

func TestGetLinks_returnsCurrentStatus__MCN_204_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{link: store.Link{Endpoint: issuerLinkID, Status: linkSignedOn}}, nil)

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
	req.Header.Set("Idempotency-Key", testLinkKey)
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
	req.Header.Set("Idempotency-Key", testLinkKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok":false`)
}

func TestPostEcho_unknownLinkIdReturns404__MCN_204_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, &fakeTrigger{})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/switch/echo", nil)
	req.Header.Set("Idempotency-Key", testLinkKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPostSignOn_returnsUpdatedLink__MCN_204_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{link: store.Link{Endpoint: issuerLinkID, Status: linkSignedOn}}, &fakeTrigger{})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/issuer/sign-on", nil)
	req.Header.Set("Idempotency-Key", testLinkKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"SIGNED_ON"`)
}

func TestPostSignOn_conflictWhenNotConnected__MCN_204_AC3(t *testing.T) {
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{}, &fakeTrigger{signOnErr: errors.New("link is not signed on")})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/links/issuer/sign-on", nil)
	req.Header.Set("Idempotency-Key", testLinkKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}

const (
	linkSignedOn    = "SIGNED_ON"
	rotationRunning = "RUNNING"
	keyActive       = "ACTIVE"
	keyZPK          = "ZPK"
	fixtureMID      = "GOCPHO000000001"

	testLinkKey  = "7d2f7a4e-3c1b-4f0e-9b8a-2a6c5d4e3f21" // a random UUID, not a secret #gitleaks:allow
	otherLinkKey = "0b1c2d3e-4f50-4617-8899-aabbccddeeff" // a random UUID, not a secret #gitleaks:allow
	signOnPath   = "/v1/network/links/issuer/sign-on"
)

func serve(t *testing.T, r http.Handler, method, target, key string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	return rec.Code, got
}

func TestLinkActions_requireAUUIDIdempotencyKey__NET_G14(t *testing.T) {
	r := chi.NewRouter()
	trigger := &fakeTrigger{}
	MountNetwork(r, &fakeLinkReader{}, trigger)

	for _, action := range []string{"echo", "sign-on", "sign-off"} {
		for _, key := range []string{"", "not-a-uuid"} {
			code, got := serve(t, r, http.MethodPost, "/v1/network/links/issuer/"+action, key)
			require.Equal(t, http.StatusBadRequest, code, action)
			require.Equal(t, "insufficient-idempotency-key", got["type"], action)
		}
	}
	require.Zero(t, trigger.signOns)
}

func TestSignOn_replaysForTheSameKeyAndRejectsReuseOnAnotherAction__NET_G14(t *testing.T) {
	r := chi.NewRouter()
	trigger := &fakeTrigger{}
	MountNetwork(r, &fakeLinkReader{link: store.Link{Endpoint: issuerLinkID, Status: linkSignedOn}}, trigger)

	code, _ := serve(t, r, http.MethodPost, signOnPath, testLinkKey)
	require.Equal(t, http.StatusOK, code)
	code, got := serve(t, r, http.MethodPost, signOnPath, testLinkKey)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, linkSignedOn, got["status"])
	require.Equal(t, 1, trigger.signOns, "a replay does not sign on again")

	code, got = serve(t, r, http.MethodPost, "/v1/network/links/issuer/sign-off", testLinkKey)
	require.Equal(t, http.StatusUnprocessableEntity, code)
	require.Equal(t, "idempotency-key-mismatch", got["type"])

	code, _ = serve(t, r, http.MethodPost, signOnPath, otherLinkKey)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, trigger.signOns)
}

func TestGetNetworkEvents_honoursLimitAndCursor__NET_G12(t *testing.T) {
	r := chi.NewRouter()
	reader := &fakeLinkReader{nextCursor: "41"}
	MountNetwork(r, reader, nil)

	code, got := serve(t, r, http.MethodGet, "/v1/network/events?limit=2&cursor=57", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, reader.gotLimit)
	require.Equal(t, "57", reader.gotCursor)
	require.Equal(t, "41", got["nextCursor"])
	require.Equal(t, "LINK_UP", got["items"].([]any)[0].(map[string]any)["code"])

	_, _ = serve(t, r, http.MethodGet, "/v1/network/events", "")
	require.Equal(t, 50, reader.gotLimit)

	for _, bad := range []string{"limit=0", "limit=201", "limit=x"} {
		code, got = serve(t, r, http.MethodGet, "/v1/network/events?"+bad, "")
		require.Equal(t, http.StatusBadRequest, code, bad)
		require.Equal(t, "validation-error", got["type"], bad)
	}
	reader.cursorErr = store.ErrInvalidCursor
	code, got = serve(t, r, http.MethodGet, "/v1/network/events?cursor=zzz", "")
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "validation-error", got["type"])
}

func TestGetLinks_carriesLiveLatencyAndInFlight__NET_G16(t *testing.T) {
	p99 := 42
	r := chi.NewRouter()
	MountNetwork(r, &fakeLinkReader{link: store.Link{Endpoint: issuerLinkID, Status: linkSignedOn}}, &fakeTrigger{p99: &p99, inFlight: 3})

	code, _ := serve(t, r, http.MethodGet, "/v1/network/links", "")
	require.Equal(t, http.StatusOK, code)
	req := httptest.NewRequest(http.MethodGet, "/v1/network/links", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Contains(t, rec.Body.String(), `"p99LatencyMs":42`)
	require.Contains(t, rec.Body.String(), `"inFlight":3`)
}
