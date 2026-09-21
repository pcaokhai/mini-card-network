# MCN-005-GW Gateway service skeleton — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development with superpowers:test-driven-development. Worktree `../mcn-worktrees/gw-005`, branch `feat/MCN-005-gw-skeleton`. Requires MCN-002 merged. Runs in parallel with MCN-005-ISS and MCN-005-SET (disjoint directories).

**Goal:** A runnable `gateway-go` with `/health/live`, `/health/ready`, Prometheus metrics on :9464, JSON logs with PAN masking and trace ids, OpenTelemetry tracing, and graceful shutdown ≤ 30 s.

**Architecture:** Thin `cmd/gateway/main.go` calling a testable `run(ctx, cfg, logger)`. Packages `internal/config` (typed env config), `internal/obs` (masking, logger, tracing), `internal/api` (chi router with health endpoints wrapped by otelhttp). Two HTTP servers (API :8080, metrics :9464) supervised by an `errgroup`.

**Tech stack:** Go (toolchain resolved at execution), chi, prometheus/client_golang, OpenTelemetry Go SDK + OTLP gRPC exporter + otelhttp, errgroup, testify, goleak, golangci-lint v2.

## Understanding

MCN-005 AC (docs/06 §E0): (1) `/health/live`, `/health/ready`, Prometheus metrics; (2) JSON logs with fields from docs/02 §7.6, masker installed and unit-tested with a PAN; (3) a request produces a trace in Tempo; (4) SIGTERM ⇒ stop accepting, drain ≤ 30 s, exit 0. Ports from docs/02 §8. Go rules in `gateway-go/CLAUDE.md`.

**Not executed in the planning sandbox** (no Go toolchain there). Every task has a verification command; run it and fix with superpowers:systematic-debugging before moving on.

## Rulings

- AC3 is verified by sending a request with a W3C `traceparent` straight to the service and finding that trace id in Tempo; the BFF hop is added when the first BFF route exists (MCN-303/305). Cost: the BFF span is proven one sprint later.
- The masker replaces any standalone run of 13–19 digits with first 6 + `*` + last 4. It does not touch 12-digit RRNs, 6-digit STANs or the 42-digit DE 90. Cost: false positives on other long numbers (acceptable in logs).
- Readiness has no dependency checks yet (DB arrives in MCN-303); it turns 503 as soon as draining starts, so load balancers stop sending traffic before shutdown.

## File map

| Action | Path (under `gateway-go/`) |
| --- | --- |
| Create | `go.mod`, `Makefile`, `.golangci.yml`, `Dockerfile`, `compose.yaml` |
| Create | `internal/config/config.go`, `internal/config/config_test.go` |
| Create | `internal/obs/mask.go`, `mask_test.go`, `logger.go`, `logger_test.go`, `tracing.go` |
| Create | `internal/api/health.go`, `health_test.go` |
| Create | `cmd/gateway/main.go`, `cmd/gateway/run_test.go` |

---

### Task 1: Module and tooling

```bash
mkdir -p gateway-go && cd gateway-go
go mod init github.com/mcn/gateway-go
go get github.com/go-chi/chi/v5@latest github.com/prometheus/client_golang@latest \
  go.opentelemetry.io/otel@latest go.opentelemetry.io/otel/sdk@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest \
  go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@latest \
  golang.org/x/sync@latest github.com/stretchr/testify@latest go.uber.org/goleak@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

`Makefile`:

```makefile
SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: test lint fmt seed run

test:
	go test -race -count=1 ./...

lint:
	test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run make fmt"; exit 1; }
	golangci-lint run ./...

fmt:
	gofmt -w .
	golangci-lint fmt ./...

seed:
	@echo "acquirer seed arrives with MCN-303"

run:
	go run ./cmd/gateway
```

`.golangci.yml` (golangci-lint v2 format; verify with `golangci-lint config verify`):

```yaml
version: "2"
linters:
  default: standard
  enable: [revive, gocyclo, gosec, bodyclose, contextcheck, errorlint, goconst]
  settings:
    gocyclo:
      min-complexity: 10
formatters:
  enable: [gofmt, goimports]
```

Commit: `build(gw): go module and tooling (MCN-005)`.

### Task 2: Typed configuration

**Step 1 — failing test** `internal/config/config_test.go`

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_defaults__MCN_005_AC1(t *testing.T) {
	cfg, err := Load(env(nil))
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, ":9464", cfg.MetricsAddr)
	require.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	require.False(t, cfg.TracingEnabled)
}

func TestLoad_overrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"HTTP_ADDR": ":18080", "SHUTDOWN_TIMEOUT": "5s", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4317",
	}))
	require.NoError(t, err)
	require.Equal(t, ":18080", cfg.HTTPAddr)
	require.Equal(t, 5*time.Second, cfg.ShutdownTimeout)
	require.True(t, cfg.TracingEnabled)
}

func TestLoad_rejectsInvalidValues(t *testing.T) {
	_, err := Load(env(map[string]string{"SHUTDOWN_TIMEOUT": "soon"}))
	require.ErrorContains(t, err, "SHUTDOWN_TIMEOUT")
	_, err = Load(env(map[string]string{"SHUTDOWN_TIMEOUT": "0s"}))
	require.ErrorContains(t, err, "must be positive")
}
```

Run: `go test ./internal/config/` → fails (`undefined: Load`).

**Step 2 — implement** `internal/config/config.go`

```go
// Package config loads and validates gateway configuration from the environment (fail fast on start).
package config

import (
	"errors"
	"fmt"
	"time"
)

// Config is the validated runtime configuration.
type Config struct {
	ServiceName     string
	HTTPAddr        string
	MetricsAddr     string
	ShutdownTimeout time.Duration
	TracingEnabled  bool
}

// Load reads configuration through getenv (os.Getenv in production, a map in tests).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ServiceName:    valueOr(getenv("OTEL_SERVICE_NAME"), "gateway"),
		HTTPAddr:       valueOr(getenv("HTTP_ADDR"), ":8080"),
		MetricsAddr:    valueOr(getenv("METRICS_ADDR"), ":9464"),
		TracingEnabled: getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
	}
	timeout, err := time.ParseDuration(valueOr(getenv("SHUTDOWN_TIMEOUT"), "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return Config{}, errors.New("SHUTDOWN_TIMEOUT must be positive")
	}
	cfg.ShutdownTimeout = timeout
	return cfg, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
```

Run: `go test ./internal/config/` → PASS. Commit: `feat(gw): typed configuration (MCN-005)`.

### Task 3: PAN masker (AC2)

**Step 1 — failing test** `internal/obs/mask_test.go`

```go
package obs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskPAN__MCN_005_AC2(t *testing.T) {
	cases := map[string]string{
		"card 9704360000004417 approved": "card 970436******4417 approved",
		"pan=4111111111111111111":        "pan=411111*********1111",
		"rrn 626514000123 stan 000123":   "rrn 626514000123 stan 000123",
		"de90 020000012409210732440000097049900000000000": "de90 020000012409210732440000097049900000000000",
		"amount 000000250000":            "amount 000000250000",
	}
	for in, want := range cases {
		require.Equal(t, want, MaskPAN(in), in)
	}
}
```

Run: `go test ./internal/obs/` → fails (`undefined: MaskPAN`).

**Step 2 — implement** `internal/obs/mask.go`

```go
// Package obs holds logging, masking and tracing setup shared by the gateway binaries.
package obs

import (
	"regexp"
	"strings"
)

// panLike matches a standalone run of 13–19 digits (PAN lengths). Longer or shorter runs are left alone.
var panLike = regexp.MustCompile(`\b\d{13,19}\b`)

// MaskPAN keeps the first 6 and last 4 digits of anything that looks like a PAN (PCI DSS 3.4).
func MaskPAN(s string) string {
	return panLike.ReplaceAllStringFunc(s, func(m string) string {
		return m[:6] + strings.Repeat("*", len(m)-10) + m[len(m)-4:]
	})
}
```

Run: `go test ./internal/obs/` → PASS. Commit: `feat(gw): PAN masker (MCN-005)`.

### Task 4: JSON logger with masking and trace ids (AC2)

**Step 1 — failing test** `internal/obs/logger_test.go`

```go
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
```

Run: `go test ./internal/obs/` → fails (`undefined: NewLogger`).

**Step 2 — implement** `internal/obs/logger.go`

```go
package obs

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger returns a JSON logger that masks PAN-like values in every string attribute and the message,
// renames "time" to "ts", and adds trace_id/span_id from the context (docs/02 §7.6).
func NewLogger(w io.Writer, service string, level slog.Level) *slog.Logger {
	json := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			if a.Value.Kind() == slog.KindString {
				a.Value = slog.StringValue(MaskPAN(a.Value.String()))
			}
			return a
		},
	})
	return slog.New(traceHandler{Handler: json}).With("service", service)
}

type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{Handler: h.Handler.WithGroup(name)}
}
```

Run: `go test ./internal/obs/` → PASS. Commit: `feat(gw): masked JSON logger with trace ids (MCN-005)`.

### Task 5: Tracing setup (AC3)

**Files:** Create `internal/obs/tracing.go` (exercised by the run test in Task 7 and by the Tempo check in Task 9).

```go
package obs

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SetupTracing installs W3C propagation and a tracer provider. With enabled=false spans are created
// (so trace ids still reach logs) but nothing is exported. The exporter reads OTEL_EXPORTER_OTLP_* env vars.
func SetupTracing(ctx context.Context, service string, enabled bool) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", service)))
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if enabled {
		exp, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
```

Run: `go build ./...` → ok. Commit: `feat(gw): OpenTelemetry tracing setup (MCN-005)`.

### Task 6: Health endpoints with draining (AC1, AC4)

**Step 1 — failing test** `internal/api/health_test.go`

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealth__MCN_005_AC1(t *testing.T) {
	health := NewHealth()
	router := NewRouter(health)

	require.Equal(t, http.StatusOK, get(t, router, "/health/live").Code)
	ready := get(t, router, "/health/ready")
	require.Equal(t, http.StatusOK, ready.Code)
	require.JSONEq(t, `{"status":"UP"}`, ready.Body.String())
}

func TestHealth_readyTurnsDownWhileDraining__MCN_005_AC4(t *testing.T) {
	health := NewHealth()
	router := NewRouter(health)

	health.StartDraining()

	require.Equal(t, http.StatusOK, get(t, router, "/health/live").Code)
	ready := get(t, router, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, ready.Code)
	require.JSONEq(t, `{"status":"DRAINING"}`, ready.Body.String())
}
```

Run: `go test ./internal/api/` → fails.

**Step 2 — implement** `internal/api/health.go`

```go
// Package api exposes the gateway's HTTP interface.
package api

import (
	"net/http"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Health tracks readiness; it becomes not-ready when graceful shutdown starts.
type Health struct{ draining atomic.Bool }

// NewHealth returns a ready Health.
func NewHealth() *Health { return &Health{} }

// StartDraining makes /health/ready fail so traffic stops before the server shuts down.
func (h *Health) StartDraining() { h.draining.Store(true) }

// NewRouter builds the HTTP handler; every request gets a server span (W3C traceparent honored).
func NewRouter(health *Health) http.Handler {
	r := chi.NewRouter()
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"status":"UP"}`)
	})
	r.Get("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if health.draining.Load() {
			writeJSON(w, http.StatusServiceUnavailable, `{"status":"DRAINING"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"UP"}`)
	})
	return otelhttp.NewHandler(r, "gateway-http")
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
```

Run: `go test ./internal/api/` → PASS. Commit: `feat(gw): health endpoints with draining (MCN-005)`.

### Task 7: run() with metrics server and graceful shutdown (AC1, AC4)

**Step 1 — failing test** `cmd/gateway/run_test.go`

```go
package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/mcn/gateway-go/internal/config"
)

func TestRun_stopsCleanlyOnCancel__MCN_005_AC4(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"))

	cfg := config.Config{ServiceName: "gateway-test", HTTPAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0", ShutdownTimeout: 2 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil))) }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return within the shutdown window")
	}
}
```

Run: `go test ./cmd/gateway/` → fails (`undefined: run`).

**Step 2 — implement** `cmd/gateway/main.go`

```go
// Command gateway runs the acquirer gateway.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"

	"github.com/mcn/gateway-go/internal/api"
	"github.com/mcn/gateway-go/internal/config"
	"github.com/mcn/gateway-go/internal/obs"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(2)
	}
	logger := obs.NewLogger(os.Stdout, cfg.ServiceName, slog.LevelInfo)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("gateway stopped with error", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("gateway stopped")
}

// run serves until ctx is cancelled, then drains within cfg.ShutdownTimeout (NFR-09).
func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	shutdownTracing, err := obs.SetupTracing(ctx, cfg.ServiceName, cfg.TracingEnabled)
	if err != nil {
		return err
	}
	health := api.NewHealth()
	apiServer := &http.Server{
		Handler:      api.NewRouter(health),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{Handler: metricsMux, ReadHeaderTimeout: 5 * time.Second}

	apiLn, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen api %s: %w", cfg.HTTPAddr, err)
	}
	metricsLn, err := net.Listen("tcp", cfg.MetricsAddr)
	if err != nil {
		_ = apiLn.Close()
		return fmt.Errorf("listen metrics %s: %w", cfg.MetricsAddr, err)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return serve(apiServer, apiLn) })
	g.Go(func() error { return serve(metricsServer, metricsLn) })
	g.Go(func() error {
		<-gctx.Done()
		logger.Info("draining", "timeout", cfg.ShutdownTimeout.String())
		health.StartDraining()
		sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return errors.Join(apiServer.Shutdown(sctx), metricsServer.Shutdown(sctx), shutdownTracing(sctx))
	})
	logger.Info("gateway started", "http", apiLn.Addr().String(), "metrics", metricsLn.Addr().String())
	return g.Wait()
}

func serve(s *http.Server, ln net.Listener) error {
	if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
```

Run: `make test` → all PASS with `-race`. Commit: `feat(gw): supervised servers with graceful shutdown (MCN-005)`.

### Task 8: Container and compose

`Dockerfile`:

```dockerfile
FROM golang:alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway
EXPOSE 8080 9464
ENTRYPOINT ["/gateway"]
```

`compose.yaml` (discovered by the root Makefile; paths relative to the repo root):

```yaml
services:
  gateway:
    build: { context: ./gateway-go }
    environment:
      OTEL_SERVICE_NAME: gateway
      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT}
      HTTP_ADDR: ":8080"
      METRICS_ADDR: ":9464"
    ports: ["8080:8080", "9464:9464"]
    stop_grace_period: 35s
    depends_on: [otel-collector]
```

### Task 9: End-to-end verification

```bash
make up
curl -fsS localhost:8080/health/live && curl -fsS localhost:8080/health/ready
curl -fsS localhost:9464/metrics | grep -q go_goroutines && echo metrics-ok
TRACE=$(openssl rand -hex 16)
curl -fsS -H "traceparent: 00-${TRACE}-$(openssl rand -hex 8)-01" localhost:8080/health/ready
sleep 8; curl -fsS "localhost:3200/api/traces/${TRACE}" | grep -q gateway && echo trace-in-tempo
docker compose --project-directory . -f infra/docker-compose.yml -f gateway-go/compose.yaml --env-file .env logs gateway | tail -3
time docker compose --project-directory . -f infra/docker-compose.yml -f gateway-go/compose.yaml --env-file .env stop gateway
docker compose --project-directory . -f infra/docker-compose.yml -f gateway-go/compose.yaml --env-file .env ps -a gateway   # Exit 0
```

Expected: two `{"status":"UP"}`, `metrics-ok`, `trace-in-tempo`, JSON log lines containing `ts` and `service`, `stop` well under 30 s, exit code 0. Then superpowers:verification-before-completion → superpowers:requesting-code-review → superpowers:finishing-a-development-branch. PR title: `feat(gw): service skeleton with health, metrics, logs, traces (MCN-005)`.

## AC → verification

| AC | Proof |
| --- | --- |
| MCN-005-AC1 | `health_test.go`; Task 9 curls on 8080 and 9464 |
| MCN-005-AC2 | `mask_test.go`, `logger_test.go` |
| MCN-005-AC3 | Task 9 trace lookup in Tempo (ruling: BFF hop in MCN-303/305) |
| MCN-005-AC4 | `health_test.go` draining, `run_test.go` (goleak), Task 9 stop timing and exit code |
