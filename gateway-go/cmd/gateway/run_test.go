package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/goleak"

	"github.com/mcn/gateway-go/internal/config"
)

func TestRun_stopsCleanlyOnCancel__MCN_005_AC4(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"))

	dbCtx := context.Background()
	container, err := postgres.Run(dbCtx, "postgres:16-alpine", postgres.WithDatabase("acquirer"), postgres.WithUsername("acquirer"), postgres.WithPassword("test"))
	require.NoError(t, err)
	defer func() { _ = container.Terminate(dbCtx) }() // must run before the goleak defer above, so declared after it (LIFO)
	dsn, err := container.ConnectionString(dbCtx, "sslmode=disable")
	require.NoError(t, err)

	cfg := config.Config{
		ServiceName: "gateway-test", HTTPAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0",
		ShutdownTimeout: 2 * time.Second, IssuerAddr: "127.0.0.1:0", DatabaseURL: dsn,
		LMKTestValueHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
	}
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
