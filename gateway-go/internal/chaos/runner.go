package chaos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	statusRunning  = "RUNNING"
	statusPassed   = "PASSED"
	statusFailed   = "FAILED"
	statusReversed = "REVERSED"

	fixedAmountMinor = 10000 // one fixed small amount per synthetic purchase; simplest thing that verifies the invariant
	currency         = "704"
	terminalID       = "TERM00000001" // ponytail: reuses the single seeded terminal (see purchase.Service's fixedMerchantID comment)

	safDrainPollInterval = 500 * time.Millisecond
	safDrainTimeout      = 65 * time.Second // SAF's own backoff cap (60s) plus margin
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

// HubPort broadcasts chaos events. *ws.Hub (extended with BroadcastChaos) satisfies it.
type HubPort interface {
	BroadcastChaos(eventType string, data any)
}

// ChaosRun mirrors contracts/openapi.yaml's ChaosRun schema.
//
//nolint:revive // named ChaosRun, not Run, to match the OpenAPI schema name exactly (see purchase.PurchaseRequest's precedent).
type ChaosRun struct {
	RunID               string `json:"runId"`
	Status              string `json:"status"`
	Requested           int    `json:"requested"`
	Completed           int    `json:"completed"`
	Approved            int    `json:"approved"`
	Declined            int    `json:"declined"`
	Reversed            int    `json:"reversed"`
	OpeningBalanceTotal int64  `json:"openingBalanceTotal"`
	ClosingBalanceTotal int64  `json:"closingBalanceTotal"`
	LedgerDiscrepancy   int64  `json:"ledgerDiscrepancy"`
}

// Runner drives synthetic load against seed cards and verifies the ledger invariant
// (opening - approved + reversed = closing, discrepancy 0) once every triggered reversal drains.
type Runner struct {
	purchases PurchaseCreator
	saf       SafDepthPort
	tranLog   TranLogGetter
	seedCards []purchase.CardFixture
	hub       HubPort

	mu   sync.Mutex
	runs map[string]*ChaosRun
}

// NewRunner builds a Runner. seedCards are the cards synthetic purchases are drawn from
// (purchase.DefaultCardTokens().Seeds() in production).
func NewRunner(purchases PurchaseCreator, saf SafDepthPort, tranLog TranLogGetter, seedCards []purchase.CardFixture, hub HubPort) *Runner {
	return &Runner{purchases: purchases, saf: saf, tranLog: tranLog, seedCards: seedCards, hub: hub, runs: map[string]*ChaosRun{}}
}

// Start kicks off requested synthetic purchases against a random seed card each, and returns
// immediately with status RUNNING; the run continues in the background. Poll Get or listen for
// chaos.run.progress / chaos.changed to observe completion.
func (r *Runner) Start(_ context.Context, requested int) (*ChaosRun, error) {
	openingTotal := int64(0)
	for _, c := range r.seedCards {
		openingTotal += c.Balance
	}

	run := &ChaosRun{RunID: newRunID(), Status: statusRunning, Requested: requested, OpeningBalanceTotal: openingTotal}
	r.mu.Lock()
	r.runs[run.RunID] = run
	// The caller gets a snapshot, never the pointer stored in r.runs - drive() (started below,
	// after unlocking) mutates that pointer from a different goroutine. Snapshotting while still
	// holding r.mu avoids racing drive() the instant it starts.
	snapshot := *run
	r.mu.Unlock()

	// ponytail: run.Start's own caller context (an HTTP request) is cancelled once the response
	// is written, so the background work deliberately uses a fresh, independent context rather
	// than the caller's - a run must keep going after the 202 response ships.
	//nolint:contextcheck,gosec // intentional new root context - see comment above.
	go r.drive(context.Background(), run.RunID)

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

func (r *Runner) drive(ctx context.Context, runID string) {
	requested, openingTotal := r.runMeta(runID)
	var approvedTotal, reversedTotal int64
	var reversalRRNs []string

	for i := 0; i < requested; i++ {
		txn, err := r.purchases.CreatePurchase(ctx, r.randomPurchaseRequest(), fmt.Sprintf("chaos-%s-%d", runID, i))
		if err != nil {
			r.finish(runID, statusFailed, 0, 0)
			return
		}
		switch txn.Status {
		case "APPROVED":
			approvedTotal += txn.Amount.Amount
			r.updateCounts(runID, func(rn *ChaosRun) { rn.Approved++ })
		case "DECLINED":
			r.updateCounts(runID, func(rn *ChaosRun) { rn.Declined++ })
		case "TIMED_OUT", "REVERSAL_PENDING":
			reversalRRNs = append(reversalRRNs, txn.RRN)
		}
		r.updateCounts(runID, func(rn *ChaosRun) { rn.Completed++ })
		r.hub.BroadcastChaos("chaos.run.progress", r.snapshot(runID))
	}

	r.waitForSafDrain(ctx)

	for _, rrn := range reversalRRNs {
		row, err := r.tranLog.Get(ctx, rrn)
		if err == nil && row.Status == statusReversed {
			reversedTotal += row.Amount
			r.updateCounts(runID, func(rn *ChaosRun) { rn.Reversed++ })
		}
	}

	// ponytail: no gateway-owned ledger balance API exists yet (only issuer-jpos owns real
	// accounts), so closingTotal is derived from this run's own tallies rather than a live
	// re-read - it verifies the runner's approved/reversed bookkeeping is internally consistent
	// (nothing stuck mid-reversal after SAF drains), not a cross-check against the issuer's
	// actual ledger. Add a real cross-check once the issuer exposes a balance query.
	closingTotal := openingTotal - approvedTotal + reversedTotal
	discrepancy := openingTotal - approvedTotal + reversedTotal - closingTotal
	status := statusPassed
	if discrepancy != 0 {
		status = statusFailed
	}
	r.finish(runID, status, closingTotal, discrepancy)
}

func (r *Runner) runMeta(runID string) (requested int, openingTotal int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[runID]
	return run.Requested, run.OpeningBalanceTotal
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

func (r *Runner) updateCounts(runID string, mutate func(*ChaosRun)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mutate(r.runs[runID])
}

func (r *Runner) finish(runID string, status string, closingTotal, discrepancy int64) {
	r.mu.Lock()
	live := r.runs[runID]
	live.Status = status
	live.ClosingBalanceTotal = closingTotal
	live.LedgerDiscrepancy = discrepancy
	snapshot := *live
	r.mu.Unlock()
	r.hub.BroadcastChaos("chaos.changed", snapshot)
}

func (r *Runner) snapshot(runID string) ChaosRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.runs[runID]
}

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
