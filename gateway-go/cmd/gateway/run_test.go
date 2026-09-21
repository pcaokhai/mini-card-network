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
