package chaos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
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

	safDrainPollInterval = 500 * time.Millisecond
	safDrainTimeout      = 65 * time.Second // SAF's own backoff cap (60s) plus margin
)

// ErrRunInProgress is returned by Start while another run is RUNNING or VERIFYING: two runs at
// once would move the same seed cards and make each run's money check meaningless.
var ErrRunInProgress = errors.New("a chaos run is already in progress")

// fixtureCardRefs maps each seed cardToken to the issuer's cardRef (contracts/fixtures/cards.json;
// a test keeps the two in sync). The gateway only knows tokens; the Issuer Admin API only refs.
var fixtureCardRefs = map[string]string{
	"tok_normal":  "crd_normal0001",
	"tok_low":     "crd_lowbal0002",
	"tok_blocked": "crd_blockd0003",
	"tok_expired": "crd_expird0004",
	"tok_limit":   "crd_limit00005",
	"tok_second":  "crd_second0006",
}

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
	LedgerBalance(ctx context.Context, cardRef string) (int64, error)
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

	mu     sync.Mutex
	runs   map[string]*ChaosRun
	order  []string // run ids, oldest first
	active string   // id of the RUNNING/VERIFYING run, "" when idle
}

// NewRunner builds a Runner. seedCards are the cards synthetic purchases are drawn from
// (purchase.DefaultCardTokens().Seeds() in production).
func NewRunner(purchases PurchaseCreator, saf SafDepthPort, tranLog TranLogGetter, balances BalanceReader, seedCards []purchase.CardFixture, hub HubPort) *Runner {
	return &Runner{purchases: purchases, saf: saf, tranLog: tranLog, balances: balances, seedCards: seedCards, hub: hub, runs: map[string]*ChaosRun{}}
}

// Start kicks off requested synthetic purchases against a random seed card each, and returns
// immediately with status RUNNING; the run continues in the background. Poll Get or listen for
// chaos.run.progress to observe completion. Returns ErrRunInProgress while another run is active.
func (r *Runner) Start(_ context.Context, requested int) (*ChaosRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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

	// ponytail: run.Start's own caller context (an HTTP request) is cancelled once the response
	// is written, so the background work deliberately uses a fresh, independent context rather
	// than the caller's - a run must keep going after the 202 response ships.
	//nolint:contextcheck,gosec // intentional new root context - see comment above.
	go r.drive(context.Background(), run.RunID, requested)

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

func (r *Runner) drive(ctx context.Context, runID string, requested int) {
	opening, err := r.balanceTotal(ctx)
	if err != nil {
		r.failRun(runID, fmt.Errorf("read opening balances: %w", err))
		return
	}
	r.update(runID, func(rn *ChaosRun) { rn.OpeningBalanceTotal = opening })

	approvedTotal, reversalRRNs, err := r.fire(ctx, runID, requested)
	if err != nil {
		r.failRun(runID, err)
		return
	}

	r.update(runID, func(rn *ChaosRun) { rn.Status = statusVerifying })
	r.waitForSafDrain(ctx)
	if err := r.countReversed(ctx, runID, reversalRRNs); err != nil {
		r.failRun(runID, err)
		return
	}
	closing, err := r.balanceTotal(ctx)
	if err != nil {
		r.failRun(runID, fmt.Errorf("read closing balances: %w", err))
		return
	}
	r.verify(runID, opening, closing, approvedTotal)
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
		if txn.Status == "TIMED_OUT" || txn.Status == "REVERSAL_PENDING" {
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

func (r *Runner) countReversed(ctx context.Context, runID string, rrns []string) error {
	for _, rrn := range rrns {
		row, err := r.tranLog.Get(ctx, rrn)
		if err != nil {
			return fmt.Errorf("read tran_log rrn=%s: %w", rrn, err)
		}
		if row.Status == statusReversed {
			r.update(runID, func(rn *ChaosRun) { rn.Reversed++ })
		}
	}
	return nil
}

// verify compares the issuer's balance movement with what the run's outcomes say it should be.
// Only approvals move money: a timed-out purchase is either never booked or booked and reversed,
// so it nets to 0 once SAF drains - and if its reversal never landed, that is exactly the
// mismatch this check exists to catch.
func (r *Runner) verify(runID string, opening, closing, approvedTotal int64) {
	expected := -approvedTotal
	discrepancy := (closing - opening) - expected
	r.update(runID, func(rn *ChaosRun) {
		rn.ClosingBalanceTotal = closing
		rn.LedgerDiscrepancy = discrepancy
		rn.Status = statusPassed
		if discrepancy != 0 {
			rn.Status = statusFailed
			rn.FailureKind = ptr(failureLedgerMismatch)
			rn.FailureDetail = ptr(fmt.Sprintf("issuer balances moved by %d, run outcomes expect %d", closing-opening, expected))
		}
	})
}

// failRun ends the run on an infrastructure error: not a money verdict (CHA-G3). The detail goes
// out in API responses, so it is PAN-masked like any log line.
func (r *Runner) failRun(runID string, err error) {
	r.update(runID, func(rn *ChaosRun) {
		rn.Status = statusFailed
		rn.FailureKind = ptr(failureRunError)
		rn.FailureDetail = ptr(obs.MaskPAN(err.Error()))
	})
}

func (r *Runner) balanceTotal(ctx context.Context) (int64, error) {
	var total int64
	for _, c := range r.seedCards {
		ref, ok := fixtureCardRefs[c.CardToken]
		if !ok {
			return 0, fmt.Errorf("no cardRef for seed card %s", c.CardToken)
		}
		balance, err := r.balances.LedgerBalance(ctx, ref)
		if err != nil {
			return 0, fmt.Errorf("card %s: %w", ref, err)
		}
		total += balance
	}
	return total, nil
}

func (r *Runner) waitForSafDrain(ctx context.Context) {
	deadline := time.Now().Add(safDrainTimeout)
	for time.Now().Before(deadline) {
		_, depth, err := r.saf.ListPending(ctx)
		if err == nil && depth == 0 {
			return
		}
		time.Sleep(safDrainPollInterval)
	}
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
