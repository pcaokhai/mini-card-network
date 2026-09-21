package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestLogger_writesMaskedJSONWithStandardFields__MCN_005_AC2(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "gateway", slog.LevelInfo)

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(),
		trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))

	log.InfoContext(ctx, "authorized 9704360000004417", "pan", "9704360000004417", "rrn", "626514000123")

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	require.Contains(t, line, "ts")
	require.Equal(t, "gateway", line["service"])
	require.Equal(t, "authorized 970436******4417", line["msg"])
	require.Equal(t, "970436******4417", line["pan"])
	require.Equal(t, "626514000123", line["rrn"])
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", line["trace_id"])
	require.Equal(t, "00f067aa0ba902b7", line["span_id"])
	require.NotContains(t, buf.String(), "9704360000004417")
}
