package chaos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	statusRunning   = "RUNNING"
	statusVerifying = "VERIFYING"
	statusPassed    = "PASSED"
	statusFailed    = "FAILED"
	statusReversed  = "REVERSED"

	failureLedgerMismatch = "LEDGER_MISMATCH"
	failureRunError       = "RUN_ERROR"

	eventRunProgress = "chaos.run.progress"

	fixedAmountMinor = 10000 // one fixed small amount per synthetic purchase; simplest thing that verifies the invariant
	currency         = "704"
	terminalID       = "00000042" // migration 00002's terminal, always present even before `make seed`

	safDrainPollInterval   = 500 * time.Millisecond
	defaultSafDrainTimeout = 65 * time.Second // SAF's own backoff cap (60s) plus margin
	defaultMaxRunDuration  = 10 * time.Minute
)

var (
	// ErrRunInProgress is returned by Start while another run is RUNNING or VERIFYING: two runs
	// at once would move the same seed cards and make each run's money check meaningless.
	ErrRunInProgress = errors.New("a chaos run is already in progress")
	// ErrNotServing is returned by Start before Serve runs or once shutdown began.
	ErrNotServing = errors.New("chaos runner is not serving")
)

// PurchaseCreator is the port Runner needs to fire synthetic purchases. *purchase.Service
// satisfies it.
type PurchaseCreator interface {
	CreatePurchase(ctx context.Context, req purchase.PurchaseRequest, idempotencyKey string) (purchase.Transaction, error)
}

// SafDepthPort reports how many saf_queue rows are still pending, so a run knows when every
// reversal it triggered has finished draining. *store.SafRepository satisfies it.
type SafDepthPort interface {
	ListPending(ctx context.Context) ([]store.SafRow, int, error)
}

// TranLogGetter looks up a transaction's final tran_log row by RRN, needed because a purchase
// that times out returns TIMED_OUT synchronously - its eventual REVERSED status only lands in
// tran_log once SAF's worker delivers the queued 0420 (purchase.Service's TranLogGetter already
// implements this).
type TranLogGetter interface {
	Get(ctx context.Context, rrn string) (store.TranLogRow, error)
}

// BalanceReader reads a card's ledger balance from the issuer, the independent side of the
// money check (CHA-G1). *IssuerAdminClient satisfies it.
type BalanceReader interface {
	LedgerBalance(ctx context.Context, cardRef string) (amountMinor int64, currency string, err error)
}

// HubPort broadcasts chaos events. *ws.Hub (extended with BroadcastChaos) satisfies it.
type HubPort interface {
	BroadcastChaos(eventType string, data any)
}

// ChaosRun mirrors contracts/openapi.yaml's ChaosRun schema.
//
//nolint:revive // named ChaosRun, not Run, to match the OpenAPI schema name exactly (see purchase.PurchaseRequest's precedent).
type ChaosRun struct {
	RunID               string    `json:"runId"`
	Status              string    `json:"status"`
	Requested           int       `json:"requested"`
	Completed           int       `json:"completed"`
	Approved            int       `json:"approved"`
	Declined            int       `json:"declined"`
	Reversed            int       `json:"reversed"`
	OpeningBalanceTotal int64     `json:"openingBalanceTotal"`
	ClosingBalanceTotal int64     `json:"closingBalanceTotal"`
	LedgerDiscrepancy   int64     `json:"ledgerDiscrepancy"`
	FailureKind         *string   `json:"failureKind"`
	FailureDetail       *string   `json:"failureDetail"`
	StartedAt           time.Time `json:"startedAt"`
	Seq                 int       `json:"seq"`
}

// Runner drives synthetic load against seed cards and verifies the ledger invariant against the
// issuer's own balances once every triggered reversal drains:
// (closing - opening) - (-Σ approved) = 0.
type Runner struct {
	purchases PurchaseCreator
	saf       SafDepthPort
	tranLog   TranLogGetter
	balances  BalanceReader
	seedCards []purchase.CardFixture
	hub       HubPort

	log             *slog.Logger
	maxRunDuration  time.Duration
	safDrainTimeout time.Duration

	mu     sync.Mutex
	runs   map[string]*ChaosRun
	order  []string        // run ids, oldest first
	active string          // id of the RUNNING/VERIFYING run, "" when idle
	base   context.Context // Serve's context; nil when not serving
	wg     sync.WaitGroup
}

// Option configures optional Runner behaviour.
type Option func(*Runner)

// WithLogger sets where a failed run's raw error is logged (slog.Default otherwise).
func WithLogger(l *slog.Logger) Option { return func(r *Runner) { r.log = l } }

// WithMaxRunDuration caps a run's wall-clock time; past it the run ends RUN_ERROR (default 10 min).
func WithMaxRunDuration(d time.Duration) Option { return func(r *Runner) { r.maxRunDuration = d } }

// WithSafDrainTimeout sets how long a run waits for the SAF queue to empty (default 65 s).
func WithSafDrainTimeout(d time.Duration) Option { return func(r *Runner) { r.safDrainTimeout = d } }

// NewRunner builds a Runner. seedCards are the cards synthetic purchases are drawn from
// (purchase.DefaultCardTokens().Seeds() in production). Runs start only while Serve runs.
func NewRunner(purchases PurchaseCreator, saf SafDepthPort, tranLog TranLogGetter, balances BalanceReader, seedCards []purchase.CardFixture, hub HubPort, opts ...Option) *Runner {
	r := &Runner{
		purchases: purchases, saf: saf, tranLog: tranLog, balances: balances, seedCards: seedCards, hub: hub,
		log: slog.Default(), maxRunDuration: defaultMaxRunDuration, safDrainTimeout: defaultSafDrainTimeout,
		runs: map[string]*ChaosRun{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Serve owns the run goroutines (root CLAUDE.md §6 rule 9): it accepts Start calls until ctx is
// cancelled, which also cancels a running run, and returns once that run has stopped.
func (r *Runner) Serve(ctx context.Context) error {
	r.mu.Lock()
	r.base = ctx
	r.mu.Unlock()
	<-ctx.Done()
	r.mu.Lock()
	r.base = nil // no Start after this point, so wg.Add never races wg.Wait
	r.mu.Unlock()
	r.wg.Wait()
	return nil
}

func (r *Runner) serving() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.base != nil
}

// Start kicks off requested synthetic purchases against a random seed card each, and returns
// immediately with status RUNNING; the run continues in the background. Poll Get or listen for
// chaos.run.progress to observe completion. Returns ErrRunInProgress while another run is active.
func (r *Runner) Start(ctx context.Context, requested int) (*ChaosRun, error) { //nolint:contextcheck // the run deliberately uses Serve's context, not the request's (see body)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.base == nil {
		return nil, ErrNotServing
	}
	if r.active != "" {
		return nil, ErrRunInProgress
	}
	run := &ChaosRun{RunID: newRunID(), Status: statusRunning, Requested: requested, StartedAt: time.Now().UTC()}
	r.runs[run.RunID] = run
	r.order = append(r.order, run.RunID)
	r.active = run.RunID
	// The caller gets a snapshot, never the pointer stored in r.runs - drive() mutates that
	// pointer from a different goroutine once it can take r.mu.
	snapshot := *run

	// The run outlives the request (cancelled once the 202 is written), so it runs under Serve's
	// context, capped at maxRunDuration, and keeps only the request's trace for its logs and the
	// issuer calls.
	runCtx, cancel := context.WithTimeout(trace.ContextWithSpanContext(r.base, trace.SpanContextFromContext(ctx)), r.maxRunDuration)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		r.drive(runCtx, run.RunID, requested)
	}()

	return &snapshot, nil
}

// Get returns a snapshot of a run's current state.
func (r *Runner) Get(runID string) (*ChaosRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok {
		return nil, false
	}
	snapshot := *run
	return &snapshot, true
}

// List returns up to limit run snapshots, newest first. Runs live in memory only (the contract
// says the list is empty after a restart).
func (r *Runner) List(limit int) []ChaosRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ChaosRun, 0, min(limit, len(r.order)))
	for i := len(r.order) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, *r.runs[r.order[i]])
	}
	return out
}

// drive runs one run to its verdict. Whatever stops it early - an error, the time cap, shutdown
// or a panic - ends it FAILED with RUN_ERROR and frees the slot for the next run.
func (r *Runner) drive(ctx context.Context, runID string, requested int) {
	defer func() {
		if p := recover(); p != nil {
			r.failRun(ctx, runID, &runError{detail: "the run stopped on an internal error", cause: fmt.Errorf("panic: %v", p)})
		}
	}()
	if err := r.execute(ctx, runID, requested); err != nil {
		r.failRun(ctx, runID, err)
	}
}

func (r *Runner) execute(ctx context.Context, runID string, requested int) error {
	if err := r.waitForSafDrain(ctx, "before the run"); err != nil {
		return err
	}
	opening, err := r.balanceTotal(ctx)
	if err != nil {
		return runErr("could not read the seed cards' opening balances from the issuer", err)
	}
	r.update(runID, func(rn *ChaosRun) { rn.OpeningBalanceTotal = opening })

	approvedTotal, reversalRRNs, err := r.fire(ctx, runID, requested)
	if err != nil {
		return runErr("a purchase could not be processed", err)
	}

	r.update(runID, func(rn *ChaosRun) { rn.Status = statusVerifying })
	if err := r.waitForSafDrain(ctx, "after the run"); err != nil {
		return err
	}
	if err := r.countReversed(ctx, runID, reversalRRNs); err != nil {
		return runErr("could not read the transaction log", err)
	}
	closing, err := r.balanceTotal(ctx)
	if err != nil {
		return runErr("could not read the seed cards' closing balances from the issuer", err)
	}
	r.verify(runID, opening, closing, approvedTotal)
	return nil
}

// fire sends the run's purchases and returns the approved amount and the RRNs left to a reversal.
func (r *Runner) fire(ctx context.Context, runID string, requested int) (int64, []string, error) {
	var approvedTotal int64
	var reversalRRNs []string
	for i := 0; i < requested; i++ {
		txn, err := r.purchases.CreatePurchase(ctx, r.randomPurchaseRequest(), fmt.Sprintf("chaos-%s-%d", runID, i))
		if err != nil {
			return 0, nil, fmt.Errorf("purchase %d of %d: %w", i+1, requested, err)
		}
		if txn.Status == "APPROVED" {
			approvedTotal += txn.Amount.Amount
		}
		if reversalQueued(txn) {
			reversalRRNs = append(reversalRRNs, txn.RRN)
		}
		r.update(runID, func(rn *ChaosRun) {
			rn.Completed++
			switch txn.Status {
			case "APPROVED":
				rn.Approved++
			case "DECLINED":
				rn.Declined++
			}
		})
	}
	return approvedTotal, reversalRRNs, nil
}

// update applies mutate, bumps seq and broadcasts the new snapshot. Only the run's own drive
// goroutine calls it, so broadcasts leave in seq order. Every state, final included, goes out as
// chaos.run.progress (chaos.changed carries a ChaosScenario, CHA-G2).
func (r *Runner) update(runID string, mutate func(*ChaosRun)) {
	r.mu.Lock()
	live := r.runs[runID]
	mutate(live)
	live.Seq++
	if (live.Status == statusPassed || live.Status == statusFailed) && r.active == runID {
		r.active = ""
	}
	snapshot := *live
	r.mu.Unlock()
	r.hub.BroadcastChaos(eventRunProgress, snapshot)
}

func ptr(s string) *string { return &s }

func (r *Runner) randomPurchaseRequest() purchase.PurchaseRequest {
	card := r.seedCards[randIntn(len(r.seedCards))]
	return purchase.PurchaseRequest{
		TerminalID: terminalID,
		CardToken:  card.CardToken,
		EntryMode:  "CHIP_PIN",
		Amount:     purchase.Money{Amount: fixedAmountMinor, Currency: currency},
	}
}

func randIntn(n int) int {
	if n <= 1 {
		return 0
	}
	i, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(i.Int64())
}

func newRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "run-" + hex.EncodeToString(b)
}
