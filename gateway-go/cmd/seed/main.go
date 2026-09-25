// Command seed fills the acquirer with a day of transactions for the Overview dashboard
// (docs/plans/MCN-002-acquirer-seed.md). It needs a running stack whose issuer is already seeded:
// every transaction is a real purchase through the gateway and the issuer's ledger. Afterwards it
// moves the gateway-side timestamps into the last hour, the rest of today and yesterday, so the
// chart and the day-over-day delta have data. The issuer's own records keep their real
// timestamps, which is why this is a lab seed and never a production tool.
//
// Environment: DATABASE_URL (acquirer DB, required), GATEWAY_URL, ISSUER_ADMIN_URL, FIXTURE,
// SEED (random seed; defaults to the clock, so every run differs).
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/mcn/gateway-go/internal/seed"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	httpTimeout   = 40 * time.Second // above the gateway's 30 s issuer timeout
	settleTimeout = 60 * time.Second
	pollInterval  = time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required (the acquirer database)")
	}
	randSeed, err := randomSeed(os.Getenv("SEED"))
	if err != nil {
		return err
	}
	fixture, err := seed.LoadFixture(envOr("FIXTURE", "../contracts/fixtures/cards.json"))
	if err != nil {
		return err
	}
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open acquirer database: %w", err)
	}
	defer pool.Close()

	runner := &seed.Runner{
		Fixture:        fixture,
		GatewayURL:     envOr("GATEWAY_URL", "http://localhost:8080"),
		IssuerAdminURL: envOr("ISSUER_ADMIN_URL", "http://localhost:8081"),
		HTTP:           &http.Client{Timeout: httpTimeout},
		Terminals:      store.NewTerminalRepository(pool),
		TranLog:        store.NewTranLogRepository(pool),
		Rand:           rand.New(rand.NewSource(randSeed)), //nolint:gosec // lab data shape, not security
		Now:            func() time.Time { return time.Now().UTC() },
		SettleTimeout:  settleTimeout,
		PollInterval:   pollInterval,
		Logf:           func(format string, args ...any) { fmt.Printf(format+"\n", args...) },
	}
	sum, err := runner.Run(ctx)
	report(sum, randSeed)
	return err
}

func report(sum seed.Summary, randSeed int64) {
	fmt.Printf("seeded %d transactions (SEED=%d): %v, %d reversed\n", sum.Planned, randSeed, sum.Outcomes, sum.Reversed)
	for _, m := range sum.Mismatches {
		fmt.Println("  unexpected outcome:", m)
	}
}

func randomSeed(raw string) (int64, error) {
	if raw == "" {
		return time.Now().UnixNano(), nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("SEED: %w", err)
	}
	return v, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
