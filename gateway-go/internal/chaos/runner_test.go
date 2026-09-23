package chaos

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

type fakePurchases struct {
	mu       sync.Mutex
	outcomes []purchase.Transaction
	calls    int
}

func (f *fakePurchases) CreatePurchase(context.Context, purchase.PurchaseRequest, string) (purchase.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	txn := f.outcomes[f.calls]
	f.calls++
	return txn, nil
}

type fakeSafDepth struct{ depth int }

func (f *fakeSafDepth) ListPending(context.Context) ([]store.SafRow, int, error) {
	rows := make([]store.SafRow, f.depth)
	return rows, f.depth, nil
}

type fakeTranLogGetter struct {
	byRRN map[string]store.TranLogRow
}

func (f *fakeTranLogGetter) Get(_ context.Context, rrn string) (store.TranLogRow, error) {
	return f.byRRN[rrn], nil
}

type fakeHub struct {
	mu     sync.Mutex
	events []string
}

func (f *fakeHub) BroadcastChaos(eventType string, _ any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, eventType)
}

func (f *fakeHub) eventCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

func TestRunner_countsApprovedDeclinedReversed__MCN_404_AC2(t *testing.T) {
	purchases := &fakePurchases{outcomes: []purchase.Transaction{
		{RRN: "r1", Status: "APPROVED", Amount: purchase.Money{Amount: 10000, Currency: "704"}},
		{RRN: "r2", Status: "DECLINED", Amount: purchase.Money{Amount: 5000, Currency: "704"}},
		{RRN: "r3", Status: "REVERSAL_PENDING", Amount: purchase.Money{Amount: 2000, Currency: "704"}},
	}}
	tranLog := &fakeTranLogGetter{byRRN: map[string]store.TranLogRow{
		"r3": {RRN: "r3", Status: "REVERSED", Amount: 2000, Currency: "704"},
	}}
	hub := &fakeHub{}
	runner := NewRunner(purchases, &fakeSafDepth{}, tranLog, []purchase.CardFixture{{CardToken: "tok_normal", PAN: "9704360000004417", Balance: 5000000}}, hub)

	run, err := runner.Start(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, "RUNNING", run.Status)

	require.Eventually(t, func() bool {
		r, _ := runner.Get(run.RunID)
		return r.Status == "PASSED" || r.Status == "FAILED"
	}, time.Second, 5*time.Millisecond)

	final, ok := runner.Get(run.RunID)
	require.True(t, ok)
	require.Equal(t, 1, final.Approved)
	require.Equal(t, 1, final.Declined)
	require.Equal(t, 1, final.Reversed)
	require.Equal(t, "PASSED", final.Status)
	require.Equal(t, int64(0), final.LedgerDiscrepancy)
	require.Equal(t, int64(5000000), final.OpeningBalanceTotal)
	require.Equal(t, int64(4992000), final.ClosingBalanceTotal) // opening - approved(10000) + reversed(2000)

	require.GreaterOrEqual(t, hub.eventCount(), 4) // 3 progress events + 1 final chaos.changed
}

func TestRunner_getUnknownRunReturnsNotOK__MCN_404_AC2(t *testing.T) {
	runner := NewRunner(&fakePurchases{}, &fakeSafDepth{}, &fakeTranLogGetter{byRRN: map[string]store.TranLogRow{}}, nil, &fakeHub{})

	_, ok := runner.Get("does-not-exist")
	require.False(t, ok)
}
