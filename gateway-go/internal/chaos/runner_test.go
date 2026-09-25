package chaos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	vnd         = "704"
	txnDeclined = "DECLINED"
)

type fakePurchases struct {
	mu       sync.Mutex
	outcomes []purchase.Transaction
	err      error
	release  chan struct{} // when set, every call blocks until it is closed or ctx ends
	calls    int
}

func (f *fakePurchases) CreatePurchase(ctx context.Context, _ purchase.PurchaseRequest, _ string) (purchase.Transaction, error) {
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return purchase.Transaction{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return purchase.Transaction{}, f.err
	}
	txn := f.outcomes[f.calls]
	f.calls++
	return txn, nil
}

// fakeSafDepth answers depths call by call first, then err if set, else depth.
type fakeSafDepth struct {
	mu     sync.Mutex
	depth  int
	depths []int
	err    error
}

func (f *fakeSafDepth) ListPending(context.Context) ([]store.SafRow, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.depth
	switch {
	case len(f.depths) > 0:
		d, f.depths = f.depths[0], f.depths[1:]
	case f.err != nil:
		return nil, 0, f.err
	}
	return make([]store.SafRow, d), d, nil
}

type fakeTranLogGetter struct {
	byRRN map[string]store.TranLogRow
}

func (f *fakeTranLogGetter) Get(_ context.Context, rrn string) (store.TranLogRow, error) {
	return f.byRRN[rrn], nil
}

// fakeBalances answers each LedgerBalance call with the next value in turn (opening reads first,
// then closing reads), or err once the values run out.
type fakeBalances struct {
	mu         sync.Mutex
	values     []int64
	currencies []string // per call, "704" when unset
	refs       []string
	err        error
}

func (f *fakeBalances) LedgerBalance(_ context.Context, cardRef string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs = append(f.refs, cardRef)
	if len(f.values) == 0 {
		return 0, "", f.err
	}
	v := f.values[0]
	f.values = f.values[1:]
	cur := vnd
	if len(f.currencies) > 0 {
		cur, f.currencies = f.currencies[0], f.currencies[1:]
	}
	return v, cur, nil
}

type fakeHub struct {
	mu        sync.Mutex
	events    []string
	snapshots []ChaosRun
}

func (f *fakeHub) BroadcastChaos(eventType string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, eventType)
	if run, ok := data.(ChaosRun); ok {
		f.snapshots = append(f.snapshots, run)
	}
}

func (f *fakeHub) recorded() ([]string, []ChaosRun) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...), append([]ChaosRun(nil), f.snapshots...)
}

var normalCard = []purchase.CardFixture{{CardToken: "tok_normal", Balance: 5000000}}

func approvedDeclinedTimedOut() *fakePurchases {
	return &fakePurchases{outcomes: []purchase.Transaction{
		{RRN: "r1", Status: "APPROVED", Amount: purchase.Money{Amount: 10000, Currency: vnd}},
		{RRN: "r2", Status: txnDeclined, Amount: purchase.Money{Amount: 5000, Currency: vnd}},
		{RRN: "r3", Status: "REVERSAL_PENDING", Amount: purchase.Money{Amount: 2000, Currency: vnd}},
	}}
}

func reversedR3() *fakeTranLogGetter {
	return &fakeTranLogGetter{byRRN: map[string]store.TranLogRow{"r3": {RRN: "r3", Status: "REVERSED", Amount: 2000, Currency: vnd}}}
}

// newServed builds a Runner under a running Serve that stops when the test ends.
func newServed(t *testing.T, purchases PurchaseCreator, saf SafDepthPort, tranLog TranLogGetter, balances BalanceReader, cards []purchase.CardFixture, hub HubPort, opts ...Option) *Runner {
	t.Helper()
	opts = append([]Option{WithSafDrainTimeout(200 * time.Millisecond)}, opts...)
	runner := NewRunner(purchases, saf, tranLog, balances, cards, hub, opts...)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Serve(ctx) }()
	require.Eventually(t, runner.serving, time.Second, 5*time.Millisecond)
	t.Cleanup(func() { cancel(); <-done })
	return runner
}

func waitFinished(t *testing.T, runner *Runner, runID string) ChaosRun {
	t.Helper()
	require.Eventually(t, func() bool {
		r, _ := runner.Get(runID)
		return r.Status == statusPassed || r.Status == statusFailed
	}, 2*time.Second, 5*time.Millisecond)
	final, _ := runner.Get(runID)
	return *final
}

func TestRunner_verifiesMoneyAgainstIssuerBalances__MCN_404_AC2_CHA_G1(t *testing.T) {
	balances := &fakeBalances{values: []int64{5000000, 4990000}} // issuer moved by exactly the one approval
	runner := newServed(t, approvedDeclinedTimedOut(), &fakeSafDepth{}, reversedR3(), balances, normalCard, &fakeHub{})

	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, statusRunning, run.Status)
	require.False(t, run.StartedAt.IsZero())

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, 1, final.Approved)
	require.Equal(t, 1, final.Declined)
	require.Equal(t, 1, final.Reversed)
	require.Equal(t, statusPassed, final.Status)
	require.Equal(t, int64(5000000), final.OpeningBalanceTotal)
	require.Equal(t, int64(4990000), final.ClosingBalanceTotal)
	require.Equal(t, int64(0), final.LedgerDiscrepancy)
	require.Nil(t, final.FailureKind)
	require.Equal(t, []string{"crd_normal0001", "crd_normal0001"}, balances.refs)
}

func TestRunner_issuerBalanceDriftIsLedgerMismatch__CHA_G1(t *testing.T) {
	// The reversal never reached the issuer's ledger: 2000 more left the card than the run approved.
	balances := &fakeBalances{values: []int64{5000000, 4988000}}
	runner := newServed(t, approvedDeclinedTimedOut(), &fakeSafDepth{}, reversedR3(), balances, normalCard, &fakeHub{})

	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, statusFailed, final.Status)
	require.Equal(t, int64(-2000), final.LedgerDiscrepancy)
	require.NotNil(t, final.FailureKind)
	require.Equal(t, "LEDGER_MISMATCH", *final.FailureKind)
}

func TestRunner_serviceErrorIsRunErrorNotLedgerMismatch__CHA_G3(t *testing.T) {
	for name, deps := range map[string]struct {
		purchases *fakePurchases
		balances  *fakeBalances
	}{
		"purchase fails":        {&fakePurchases{err: errors.New("db down")}, &fakeBalances{values: []int64{5000000}}},
		"opening balance fails": {approvedDeclinedTimedOut(), &fakeBalances{err: errors.New("issuer admin down")}},
		"closing balance fails": {approvedDeclinedTimedOut(), &fakeBalances{values: []int64{5000000}, err: errors.New("issuer admin down")}},
	} {
		t.Run(name, func(t *testing.T) {
			runner := newServed(t, deps.purchases, &fakeSafDepth{}, reversedR3(), deps.balances, normalCard, &fakeHub{})
			run, err := runner.Start(context.Background(), 3)
			require.NoError(t, err)

			final := waitFinished(t, runner, run.RunID)
			require.Equal(t, statusFailed, final.Status)
			require.Equal(t, "RUN_ERROR", *final.FailureKind)
			require.NotNil(t, final.FailureDetail)
			require.NotEmpty(t, *final.FailureDetail)
			require.Equal(t, int64(0), final.LedgerDiscrepancy)
		})
	}
}

func TestRunner_runErrorDetailIsGenericAndTheLogMasksPAN__CHA_G3_N3(t *testing.T) {
	var logs bytes.Buffer
	purchases := &fakePurchases{err: errors.New("card 9704360000004417 lookup failed: pq: relation missing")}
	runner := newServed(t, purchases, &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{1}}, normalCard, &fakeHub{},
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))))
	run, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, "a purchase could not be processed", *final.FailureDetail)
	require.Eventually(t, func() bool { return strings.Contains(logs.String(), "970436******4417") }, time.Second, 5*time.Millisecond)
	require.NotContains(t, logs.String(), "9704360000004417")
	require.Contains(t, logs.String(), run.RunID)
}

func TestRunner_safNotDrainedAfterTheRunIsRunErrorNotMismatch__CHA_S1(t *testing.T) {
	for name, saf := range map[string]*fakeSafDepth{
		"still pending": {depths: []int{0}, depth: 2},
		"read error":    {depths: []int{0}, err: errors.New("db down")},
	} {
		t.Run(name, func(t *testing.T) {
			balances := &fakeBalances{values: []int64{5000000, 4988000}}
			runner := newServed(t, approvedDeclinedTimedOut(), saf, reversedR3(), balances, normalCard, &fakeHub{})
			run, err := runner.Start(context.Background(), 3)
			require.NoError(t, err)

			final := waitFinished(t, runner, run.RunID)
			require.Equal(t, "RUN_ERROR", *final.FailureKind)
			require.Contains(t, *final.FailureDetail, "SAF not drained")
			require.Equal(t, int64(0), final.LedgerDiscrepancy)
		})
	}
}

func TestRunner_waitsForAnEmptySAFBeforeTheOpeningRead__CHA_S2(t *testing.T) {
	balances := &fakeBalances{values: []int64{5000000, 4990000}}
	saf := &fakeSafDepth{depths: []int{3, 1, 0}}
	runner := newServed(t, approvedDeclinedTimedOut(), saf, reversedR3(), balances, normalCard, &fakeHub{})
	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, statusPassed, waitFinished(t, runner, run.RunID).Status)

	stuck := &fakeBalances{values: []int64{5000000}}
	runner = newServed(t, approvedDeclinedTimedOut(), &fakeSafDepth{depth: 4}, reversedR3(), stuck, normalCard, &fakeHub{})
	run, err = runner.Start(context.Background(), 3)
	require.NoError(t, err)
	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, "RUN_ERROR", *final.FailureKind)
	require.Equal(t, "SAF not drained before the run (4 pending)", *final.FailureDetail)
	require.Empty(t, stuck.refs, "no opening read while earlier reversals are still owed")
}

func TestRunner_ledgerMismatchDetailStatesTheNoOtherTrafficAssumption__CHA_S2(t *testing.T) {
	runner := newServed(t, approvedDeclinedTimedOut(), &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{5000000, 4988000}}, normalCard, &fakeHub{})
	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)
	require.Contains(t, *waitFinished(t, runner, run.RunID).FailureDetail, "no other traffic on the seed cards")
}

func TestRunner_countsMACFailureReversals__CHA_N1(t *testing.T) {
	purchases := &fakePurchases{outcomes: []purchase.Transaction{
		{RRN: "r1", Status: txnDeclined, ResponseCode: "96", Amount: purchase.Money{Amount: 10000, Currency: vnd}},
	}}
	tranLog := &fakeTranLogGetter{byRRN: map[string]store.TranLogRow{"r1": {RRN: "r1", Status: "REVERSED", Amount: 10000}}}
	runner := newServed(t, purchases, &fakeSafDepth{}, tranLog, &fakeBalances{values: []int64{100, 100}}, normalCard, &fakeHub{})
	run, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, 1, final.Reversed)
	require.Equal(t, statusPassed, final.Status)
}

// panickyPurchases panics on the first call, standing in for a bug in a dependency.
type panickyPurchases struct{}

func (panickyPurchases) CreatePurchase(context.Context, purchase.PurchaseRequest, string) (purchase.Transaction, error) {
	panic("boom")
}

func TestRunner_aPanicEndsTheRunAndFreesTheSlot__CHA_N2(t *testing.T) {
	runner := newServed(t, panickyPurchases{}, &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{1, 1}}, normalCard, &fakeHub{})
	run, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, "RUN_ERROR", *final.FailureKind)
	_, err = runner.Start(context.Background(), 1)
	require.NotErrorIs(t, err, ErrRunInProgress)
}

func TestRunner_wallClockCapEndsTheRun__CHA_N2(t *testing.T) {
	purchases := &fakePurchases{outcomes: []purchase.Transaction{{RRN: "r1", Status: txnDeclined}}, release: make(chan struct{})}
	defer close(purchases.release)
	runner := newServed(t, purchases, &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{1, 1}}, normalCard, &fakeHub{},
		WithMaxRunDuration(50*time.Millisecond))
	run, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, "RUN_ERROR", *final.FailureKind)
	require.Contains(t, *final.FailureDetail, "time limit")
}

func TestRunner_shutdownEndsARunningRun__CHA_N2(t *testing.T) {
	purchases := &fakePurchases{outcomes: []purchase.Transaction{{RRN: "r1", Status: txnDeclined}}, release: make(chan struct{})}
	defer close(purchases.release)
	runner := NewRunner(purchases, &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{1, 1}}, normalCard, &fakeHub{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Serve(ctx) }()
	require.Eventually(t, runner.serving, time.Second, 5*time.Millisecond)
	run, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)

	cancel()
	require.NoError(t, <-done, "Serve returns once the run goroutine has stopped")
	final, _ := runner.Get(run.RunID)
	require.Equal(t, statusFailed, final.Status)
	require.Contains(t, *final.FailureDetail, "shut down")
	_, err = runner.Start(context.Background(), 1)
	require.ErrorIs(t, err, ErrNotServing)
}

func TestRunner_mixedCurrenciesAreRunError__CHA_N4(t *testing.T) {
	balances := &fakeBalances{values: []int64{1, 2}, currencies: []string{"704", "840"}}
	cards := []purchase.CardFixture{{CardToken: "tok_normal"}, {CardToken: "tok_low"}}
	runner := newServed(t, approvedDeclinedTimedOut(), &fakeSafDepth{}, reversedR3(), balances, cards, &fakeHub{})
	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)

	final := waitFinished(t, runner, run.RunID)
	require.Equal(t, "RUN_ERROR", *final.FailureKind)
	require.Contains(t, *final.FailureDetail, "currencies")
}

func TestRunner_broadcastsEveryStateAsProgressWithIncreasingSeq__CHA_G2_G8_G13(t *testing.T) {
	hub := &fakeHub{}
	runner := newServed(t, approvedDeclinedTimedOut(), &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{5000000, 4990000}}, normalCard, hub)
	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)
	final := waitFinished(t, runner, run.RunID)

	require.Eventually(t, func() bool { _, s := hub.recorded(); return len(s) > 0 && s[len(s)-1].Status == statusPassed }, time.Second, 5*time.Millisecond)
	events, snapshots := hub.recorded()
	sawVerifying := false
	for i, ev := range events {
		require.Equal(t, "chaos.run.progress", ev)
		if i > 0 {
			require.Greater(t, snapshots[i].Seq, snapshots[i-1].Seq)
		}
		sawVerifying = sawVerifying || snapshots[i].Status == "VERIFYING"
	}
	require.True(t, sawVerifying, "VERIFYING is broadcast during the SAF drain and verification")
	require.Equal(t, final.Seq, snapshots[len(snapshots)-1].Seq)
}

func TestRunner_rejectsASecondRunWhileOneIsInProgress__CHA_G5(t *testing.T) {
	declined := purchase.Transaction{RRN: "r1", Status: txnDeclined}
	purchases := &fakePurchases{outcomes: []purchase.Transaction{declined, declined}, release: make(chan struct{})}
	runner := newServed(t, purchases, &fakeSafDepth{}, reversedR3(), &fakeBalances{values: []int64{1, 1, 1, 1}}, normalCard, &fakeHub{})

	first, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)
	_, err = runner.Start(context.Background(), 1)
	require.ErrorIs(t, err, ErrRunInProgress)

	close(purchases.release)
	waitFinished(t, runner, first.RunID)
	second, err := runner.Start(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, statusPassed, waitFinished(t, runner, second.RunID).Status)
}

func TestRunner_listsRunsNewestFirst__CHA_G6(t *testing.T) {
	runner := newServed(t, &fakePurchases{err: errors.New("x")}, &fakeSafDepth{}, reversedR3(), &fakeBalances{err: errors.New("x")}, normalCard, &fakeHub{})
	var ids []string
	for range 3 {
		run, err := runner.Start(context.Background(), 1)
		require.NoError(t, err)
		waitFinished(t, runner, run.RunID)
		ids = append(ids, run.RunID)
	}

	got := runner.List(2)
	require.Len(t, got, 2)
	require.Equal(t, ids[2], got[0].RunID)
	require.Equal(t, ids[1], got[1].RunID)
	require.Len(t, runner.List(50), 3)
}

func TestRunner_getUnknownRunReturnsNotOK__MCN_404_AC2(t *testing.T) {
	runner := newServed(t, &fakePurchases{}, &fakeSafDepth{}, &fakeTranLogGetter{byRRN: map[string]store.TranLogRow{}}, &fakeBalances{}, nil, &fakeHub{})

	_, ok := runner.Get("does-not-exist")
	require.False(t, ok)
}

func TestFixtureCardRefs_matchContractFixtures__CHA_G1(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/fixtures/cards.json")
	require.NoError(t, err)
	var fixture struct {
		Cards []struct{ CardToken, CardRef string }
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	want := map[string]string{}
	for _, c := range fixture.Cards {
		want[c.CardToken] = c.CardRef
	}
	require.Equal(t, want, fixtureCardRefs)
	for _, seed := range purchase.DefaultCardTokens().Seeds() {
		require.Contains(t, fixtureCardRefs, seed.CardToken)
	}
}
