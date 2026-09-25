package chaos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestIssuerAdminClient_readsLedgerBalanceByCardRef__CHA_G1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/v1/cards/crd_normal0001", req.URL.Path)
		_, _ = w.Write([]byte(`{"cardRef":"crd_normal0001","maskedPan":"970436******4417","ledgerBalance":{"amount":4990000,"currency":"704"},"availableBalance":{"amount":4990000,"currency":"704"}}`))
	}))
	defer srv.Close()

	balance, cur, err := NewIssuerAdminClient(srv.URL).LedgerBalance(context.Background(), "crd_normal0001")
	require.NoError(t, err)
	require.Equal(t, int64(4990000), balance)
	require.Equal(t, "704", cur)
}

func TestIssuerAdminClient_non200IsAnError__CHA_G1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := NewIssuerAdminClient(srv.URL).LedgerBalance(context.Background(), "crd_missing0001")
	require.ErrorContains(t, err, "404")
}

func TestIssuerAdminClient_missingLedgerBalanceIsAnError__CHA_G1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"cardRef":"crd_normal0001"}`))
	}))
	defer srv.Close()

	_, _, err := NewIssuerAdminClient(srv.URL).LedgerBalance(context.Background(), "crd_normal0001")
	require.Error(t, err)
}

func TestIssuerAdminClient_propagatesTraceparent__CHA_N4(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got = req.Header.Get("traceparent")
		_, _ = w.Write([]byte(`{"ledgerBalance":{"amount":1,"currency":"704"}}`))
	}))
	defer srv.Close()
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))

	_, _, err := NewIssuerAdminClient(srv.URL).LedgerBalance(ctx, "crd_normal0001")
	require.NoError(t, err)
	require.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", got)
}
