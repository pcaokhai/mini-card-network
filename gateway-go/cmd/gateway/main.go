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

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"

	"github.com/mcn/gateway-go/internal/api"
	"github.com/mcn/gateway-go/internal/config"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/store"
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
	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer pool.Close()
	linkRepo := store.NewLinkRepository(pool)
	supervisor := isonet.NewSupervisor(isonet.Config{Addr: cfg.IssuerAddr, EchoInterval: 60 * time.Second, EchoFailureLimit: 3}, linkRepo)
	health := api.NewHealth()
	r := chi.NewRouter()
	api.NewRouter(r, health)
	api.MountLab(r)
	apiServer := &http.Server{
		Handler:      otelhttp.NewHandler(r, "gateway-http"),
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
	g.Go(func() error { return supervisor.Run(gctx) })
	g.Go(func() error {
		<-gctx.Done()
		logger.Info("draining", "timeout", cfg.ShutdownTimeout.String())
		health.StartDraining()
		// sctx is deliberately fresh, not derived from ctx: ctx is already Done here (that's
		// why we're draining), so Shutdown must get a context that hasn't been cancelled yet.
		sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return errors.Join(apiServer.Shutdown(sctx), metricsServer.Shutdown(sctx), shutdownTracing(sctx)) //nolint:contextcheck
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
