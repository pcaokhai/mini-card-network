package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/mcn/gateway-go/internal/store"
)

const problemTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

func tracedRequest(method, target, body string) *http.Request {
	traceID, _ := trace.TraceIDFromHex(problemTraceID)
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
	req := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func problemRouter() chi.Router {
	r := chi.NewRouter()
	MountLab(r)
	MountPurchases(r, &fakePurchaseService{})
	MountTransactionsQuery(r, &fakeTranLogReader{byRRN: map[string]store.TranLogRow{}}, fakeReversals{})
	MountOverview(r, &fakeOverviewReader{err: errors.New("pq: connection to 10.0.0.7 refused")}, testCalendar)
	return r
}

// Every gateway problem follows docs/04 §3: a full URI type from the catalogue, the request path
// as instance, the request's W3C trace id, as application/problem+json (P-1).
func TestProblems_followTheDocs04Shape__P_1(t *testing.T) {
	cases := []struct {
		name, method, target, body string
		status                     int
		slug                       string
	}{
		{"unknown transaction", http.MethodGet, "/v1/transactions/626514999999", "", http.StatusNotFound, "not-found"},
		{"bad list query", http.MethodGet, "/v1/transactions?limit=0", "", http.StatusBadRequest, problemValidation},
		{"missing idempotency key", http.MethodPost, "/v1/transactions/purchases", validPurchaseBody, http.StatusBadRequest, "insufficient-idempotency-key"},
		{"bad Lab message", http.MethodPost, "/v1/lab/messages/decode", `{"raw":"ABCD"}`, http.StatusBadRequest, problemValidation},
		{"Lab body not JSON", http.MethodPost, "/v1/lab/messages/decode", `{`, http.StatusBadRequest, problemValidation},
		{"database failure", http.MethodGet, "/v1/metrics/overview", "", http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := tracedRequest(tc.method, tc.target, tc.body)

			problemRouter().ServeHTTP(rec, req)

			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			var p map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
			require.Equal(t, "https://mcn.local/problems/"+tc.slug, p["type"])
			require.NotEmpty(t, p["title"])
			require.EqualValues(t, tc.status, p["status"])
			require.Equal(t, req.URL.Path, p["instance"])
			require.Equal(t, problemTraceID, p["traceId"])
		})
	}
}

func TestProblems_anInternalErrorCarriesNoCause__P_1(t *testing.T) {
	rec := httptest.NewRecorder()

	problemRouter().ServeHTTP(rec, tracedRequest(http.MethodGet, "/v1/metrics/overview", ""))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "10.0.0.7", "an internal 500 carries only the trace id (docs/04 §3)")
}

func TestProblems_aLabCodecErrorNamesItsCodeInErrors__P_1(t *testing.T) {
	rec := httptest.NewRecorder()

	problemRouter().ServeHTTP(rec, tracedRequest(http.MethodPost, "/v1/lab/messages/decode", `{"raw":"ABCD"}`))

	p := decodeProblem(t, rec)
	require.Equal(t, problemValidation, p.Type)
	require.Equal(t, "raw", p.Errors[0].Field)
	require.Contains(t, p.Errors[0].Message, "INVALID_")
}
