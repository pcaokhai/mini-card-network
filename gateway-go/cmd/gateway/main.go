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

	"github.com/mcn/gateway-go/internal/advtxn"
	"github.com/mcn/gateway-go/internal/api"
	"github.com/mcn/gateway-go/internal/chaos"
	"github.com/mcn/gateway-go/internal/chaos/fakeissuer"
	"github.com/mcn/gateway-go/internal/config"
	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/rotation"
	"github.com/mcn/gateway-go/internal/saf"
	"github.com/mcn/gateway-go/internal/store"
	"github.com/mcn/gateway-go/internal/ws"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(2)
	}
	logger := obs.NewLogger(os.Stdout, cfg.ServiceName, slog.LevelInfo)
	// Handlers that have no logger of their own (api.transactionProblem) log through the default.
	slog.SetDefault(logger)
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
	// A bad LMK must fail the process before it binds a port.
	hsmModule, err := hsm.NewJCEModule(cfg.LMKTestValueHex)
	if err != nil {
		return fmt.Errorf("init hsm module: %w", err)
	}
	shutdownTracing, err := obs.SetupTracing(ctx, cfg.ServiceName, cfg.TracingEnabled)
	if err != nil {
		return err
	}
	pool, err := openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	linkRepo := store.NewLinkRepository(pool)
	lastSTAN := func(ctx context.Context) (int64, error) {
		return store.NewTranLogRepository(pool).LastSTANInRRNPrefix(ctx, purchase.BuildRRN(time.Now().UTC(), ""))
	}
	supervisor := isonet.NewSupervisor(isonet.Config{Addr: cfg.IssuerAddr, EchoInterval: 60 * time.Second, EchoTimeout: cfg.EchoTimeout, EchoFailureLimit: 3, LastSTAN: lastSTAN}, linkRepo)
	hub := ws.NewHub()
	supervisor.SetHub(hub)
	tranLogRepo := store.NewTranLogRepository(pool)
	keyStoreRepo := store.NewKeyStoreRepository(pool)
	activeKeys, err := loadActiveZAK(ctx, keyStoreRepo, hsmModule, cfg, logger)
	if err != nil {
		return err
	}
	// ponytail: the services below still take the ZAK as a byte slice read once here; switching
	// them to activeKeys (hsm.ZAKSource) waits for #114/#117, which rewrite those constructors.
	zak := activeKeys.ActiveZAK()
	safRepo := store.NewSafRepository(pool)
	reversalQueuer := saf.NewReversalQueuer(pool, cfg.SafEncKey)
	terminalRepo := store.NewTerminalRepository(pool)
	// A response MAC is retried under a PENDING key of unknown outcome, else the recently retired
	// key (review S2 of #121).
	macFallback := keyStoreRepo.MACFallback()
	purchaseService := purchase.NewService(supervisor, purchase.DefaultCardTokens(), terminalRepo, tranLogRepo, store.NewIdempotencyRepository(pool), hub, reversalQueuer, hsmModule, zak, macFallback, linkRepo)
	advtxnService := advtxn.NewService(supervisor, purchase.DefaultCardTokens(), terminalRepo, tranLogRepo, store.NewIdempotencyRepository(pool), advtxnHubAdapter{hub: hub}, reversalQueuer, hsmModule, zak, macFallback)
	rotationRepo := rotation.NewRepository(pool)
	rotationRunner := rotation.NewRunner(rotationRepo, keyStoreRepo, hsmModule, supervisor, cfg.ZMK,
		rotation.WithActivationHook(reloadOnActivate(activeKeys, logger)), rotation.WithSendAttempts(cfg.RotationSendAttempts), rotation.WithSendTimeout(cfg.EchoTimeout))
	supervisor.SetLateResponseHandler(newLateResponseHandler(ctx, logger, purchaseService))
	ackAnnouncer := reversalAckAnnouncer{Port: safRepo, rrnOf: safRepo.TranRRN, announce: purchaseService.BroadcastUpdate, logger: logger}
	safWorker := saf.NewWorker(supervisor, purchase.DefaultCardTokens(), hsmModule, zak, ackAnnouncer, cfg.SafEncKey, isonet.Backoff{Base: 2 * time.Second, Cap: 60 * time.Second}, time.Second)
	safWorker.SetLogger(logger)
	// Orphans are rows older than the 30 s send timeout plus the 10 s a request takes to record
	// its outcome, with a margin (POS-G16).
	sweeper := saf.NewSweeper(tranLogRepo, reversalQueuer, 90*time.Second, 30*time.Second)
	sweeper.SetLogger(logger)

	fakeIssuer, toxiproxyOpts, err := setupFakeIssuer(cfg)
	if err != nil {
		return err
	}
	toxiproxyClient := chaos.NewToxiproxyClient(cfg.ToxiproxyAdminAddr, cfg.IssuerProxyName, toxiproxyOpts...)
	if err := toxiproxyClient.DisableAll(ctx); err != nil {
		// Non-fatal: a boot-time Toxiproxy hiccup shouldn't stop the gateway from serving real
		// traffic, only chaos scenarios.
		logger.Error("disable chaos toxics at boot", "error", err.Error())
	}
	chaosRunner := chaos.NewRunner(purchaseService, safRepo, tranLogRepo, chaos.NewIssuerAdminClient(cfg.IssuerAdminURL), purchase.DefaultCardTokens().Seeds(), hub,
		chaos.WithLogger(logger), chaos.WithMaxRunDuration(cfg.ChaosRunMaxDuration))
	purchaseService.SetChaosDuplicateHook(newChaosDuplicateHook(ctx, toxiproxyClient))

	health := api.NewHealth()
	r := chi.NewRouter()
	r.Use(api.CORSMiddleware(cfg.CORSAllowedOrigin))
	api.NewRouter(r, health)
	api.MountLab(r)
	api.MountNetwork(r, linkRepo, supervisor, safRepo)
	api.MountTerminals(r, terminalRepo)
	api.MountPurchases(r, purchaseService)
	api.MountAdvancedTransactions(r, advtxnService)
	api.MountTransactionsQuery(r, tranLogRepo, saf.NewReversalLookup(safRepo, cfg.SafEncKey))
	api.MountOverview(r, tranLogRepo)
	api.MountKeys(r, keyStoreRepo, cfg.KeyLifetimeDays)
	api.MountRotations(r, rotationAdapter{runner: rotationRunner, repo: rotationRepo})
	api.MountChaos(r, toxiproxyClient, chaosRunner, hub)
	r.Handle("/v1/stream", hub)
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
	g.Go(func() error { return safWorker.Run(gctx) })
	g.Go(func() error { return sweeper.Run(gctx) })
	g.Go(func() error { return chaosRunner.Serve(gctx) })
	g.Go(func() error { return rotationRunner.Serve(gctx) })
	if fakeIssuer != nil {
		g.Go(func() error { return fakeIssuer.Serve(gctx) })
	}
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

// openDatabase migrates, then opens the pool.
func openDatabase(ctx context.Context, url string) (*store.Pool, error) {
	if err := store.Migrate(url); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	pool, err := store.Open(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return pool, nil
}

func serve(s *http.Server, ln net.Listener) error {
	if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// provisionInitialKeys registers ZAK_HEX / ZPK_HEX as the ACTIVE working keys when key_store has
// none, so a fresh stack MACs from the first purchase. A key set by a rotation is left alone.
func provisionInitialKeys(ctx context.Context, repo *store.KeyStoreRepository, hsmModule hsm.Module, cfg config.Config, logger *slog.Logger) error {
	for _, k := range []struct {
		keyType string
		clear   []byte
	}{{"ZAK", cfg.InitialZAK}, {"ZPK", cfg.InitialZPK}} {
		got, err := rotation.ProvisionInitialKey(ctx, repo, hsmModule, k.keyType, k.clear)
		if err != nil {
			return err
		}
		switch {
		case got.Inserted:
			logger.Info("registered initial working key", "key_type", k.keyType, "kcv", got.KCV)
		case got.ActiveKCV != got.KCV:
			logger.Warn("configured working key differs from the ACTIVE one; MACs fail unless the issuer uses the ACTIVE key",
				"key_type", k.keyType, "configured_kcv", got.KCV, "active_kcv", got.ActiveKCV)
		}
	}
	return nil
}

// loadActiveZAK registers the initial working keys if needed, then loads the ACTIVE ZAK into an
// ActiveKeys that a rotation reloads (SEC-G10). Failing to register a configured key stops
// startup (config fails fast, CLAUDE.md §6.11). A missing ZAK only happens when ZAK_HEX is unset
// and no rotation has run: MAC then fails per purchase instead of blocking startup, which keeps
// the unit-level run tests key-free.
func loadActiveZAK(ctx context.Context, repo *store.KeyStoreRepository, hsmModule hsm.Module, cfg config.Config, logger *slog.Logger) (*rotation.ActiveKeys, error) {
	if err := provisionInitialKeys(ctx, repo, hsmModule, cfg, logger); err != nil {
		return nil, fmt.Errorf("register initial working keys: %w", err)
	}
	keys := rotation.NewActiveKeys(repo, hsmModule)
	if err := keys.Reload(ctx, "ZAK"); err != nil {
		logger.Warn("no active ZAK found, MAC on purchases will fail until one is provisioned", "error", err.Error())
	}
	return keys, nil
}

// reloadOnActivate swaps in a key a rotation just activated (SEC-G10). A failed reload keeps the
// previous key, which the issuer still accepts during the dual-key window, and is logged.
func reloadOnActivate(keys *rotation.ActiveKeys, logger *slog.Logger) func(context.Context, string) {
	return func(ctx context.Context, keyType string) {
		if err := keys.Reload(ctx, keyType); err != nil {
			logger.Error("reload activated key", "key_type", keyType, "error", err.Error())
		}
	}
}

// advtxnHubAdapter adapts *ws.Hub to advtxn.HubPort: ws.Hub.BroadcastTransaction is typed to
// purchase.Transaction, so advtxn.Service's own Transaction type broadcasts through ws.Hub's
// already-generic BroadcastChaos(eventType string, data any) instead (same broadcastEvent path
// on the wire; the Go method name doesn't reach the client, only the eventType string does).
type advtxnHubAdapter struct{ hub *ws.Hub }

func (a advtxnHubAdapter) BroadcastTransaction(eventType string, txn advtxn.Transaction) {
	a.hub.BroadcastChaos(eventType, txn)
}

// reversalAckAnnouncer broadcasts transaction.updated once the SAF worker's acknowledgement has
// moved a transaction to REVERSED (OVW-G3). The acknowledgement is already committed, so a failed
// announcement is only logged.
type reversalAckAnnouncer struct {
	saf.Port
	rrnOf    func(ctx context.Context, safID int64) (string, error)
	announce func(ctx context.Context, rrn string) error
	logger   *slog.Logger
}

func (a reversalAckAnnouncer) MarkAcked(ctx context.Context, id int64) error {
	if err := a.Port.MarkAcked(ctx, id); err != nil {
		return err
	}
	rrn, err := a.rrnOf(ctx, id)
	if err == nil {
		err = a.announce(ctx, rrn)
	}
	if err != nil {
		a.logger.ErrorContext(ctx, "announce acknowledged reversal", "saf_id", id, "error", err.Error())
	}
	return nil
}

// rotationAdapter adapts rotation.Runner/rotation.Repository to api.Rotator. Rotation initiation
// (owner-ref empty, matching activeClearKey's single-global-key-per-type simulator model) returns
// the RUNNING row at once; the steps run on the runner's own goroutine (SEC-G2).
type rotationAdapter struct {
	runner *rotation.Runner
	repo   *rotation.Repository
}

func (a rotationAdapter) StartRotation(ctx context.Context, keyType string) (rotation.Row, error) {
	return a.runner.Start(ctx, keyType, "")
}

func (a rotationAdapter) GetRotation(ctx context.Context, id int64) (rotation.Row, error) {
	return a.repo.Get(ctx, id)
}

// setupFakeIssuer starts the MCN-407 DROP_RESPONSE fake-issuer listener when
// cfg.ChaosFakeIssuerAddr is set, and returns the ToxiproxyClientOption that repoints the issuer
// proxy at it during that scenario. Returns a nil listener and no options when unset (off by
// default).
func setupFakeIssuer(cfg config.Config) (*fakeissuer.Listener, []chaos.ToxiproxyClientOption, error) {
	if cfg.ChaosFakeIssuerAddr == "" {
		return nil, nil, nil
	}
	fakeIssuer, err := fakeissuer.NewListener(cfg.ChaosFakeIssuerAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen chaos fake issuer %s: %w", cfg.ChaosFakeIssuerAddr, err)
	}
	// cfg.ChaosFakeIssuerAddr (not fakeIssuer.Addr()) is what Toxiproxy - a different container -
	// must dial, e.g. "gateway:19999"; fakeIssuer.Addr() is only the local bind address (e.g.
	// "[::]:19999") once net.Listen resolves it, which isn't dialable from another container.
	return fakeIssuer, []chaos.ToxiproxyClientOption{chaos.WithDropResponseAddr(cfg.ChaosFakeIssuerAddr)}, nil
}

// newLateResponseHandler builds the isonet.Supervisor callback (MCN-403) that persists a 0210
// arriving after its transaction already left SENT, without ever flipping a final status.
func newLateResponseHandler(ctx context.Context, logger *slog.Logger, purchaseService *purchase.Service) func(string, map[int]string) {
	return func(_ string, fields map[int]string) {
		if err := purchaseService.RecordLateResponse(ctx, fields[37], fields[39]); err != nil {
			logger.Error("record late response", "error", err.Error())
		}
	}
}

// newChaosDuplicateHook reports whether the MCN-404 DUPLICATE_REQUEST scenario is currently
// active, so purchase.Service knows whether to fire a blocking duplicate 0200 before the real
// send.
func newChaosDuplicateHook(ctx context.Context, toxiproxyClient *chaos.ToxiproxyClient) func() bool {
	return func() bool {
		scenarios, err := toxiproxyClient.ListScenarios(ctx)
		if err != nil {
			return false
		}
		for _, s := range scenarios {
			if s.ID == chaos.ScenarioDuplicateRequest {
				return s.Enabled
			}
		}
		return false
	}
}
