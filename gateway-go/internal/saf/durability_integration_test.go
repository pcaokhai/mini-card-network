package saf

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

// TestSafWorker_survivesRestart_deliversEveryPendingReversal simulates a worker process dying
// right after claiming a row (it flips to IN_FLIGHT but never acks/dead-letters it) - a real
// SIGKILL mid-delivery leaves saf_queue in exactly this state. A fresh Worker instance against
// the same pool must still recover and deliver it (MCN-401-AC4): ClaimDue's SQL also matches
// IN_FLIGHT rows whose next_retry_at has already passed, not just PENDING ones.
func TestSafWorker_survivesRestart_deliversEveryPendingReversal__MCN_401_AC4(t *testing.T) {
	pool := newTestPool(t)
	tranLog := store.NewTranLogRepository(pool)
	safRepo := store.NewSafRepository(pool)
	ctx := context.Background()

	tranID, err := tranLog.Insert(ctx, store.TranLogRow{RRN: "626514000999", Type: tranTypePurchase, Status: "TIMED_OUT", Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000999"})
	require.NoError(t, err)
	id, err := safRepo.Enqueue(ctx, tranID, "0420", []byte("{}"))
	require.NoError(t, err)

	// Simulate a worker that died right after claiming: ClaimDue flips it to IN_FLIGHT, and
	// nothing ever acks or dead-letters it.
	claimed, err := safRepo.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, id, claimed[0].ID)

	// A fresh Worker instance recovers the stale IN_FLIGHT row and delivers it.
	mux := &fakeMux{response: map[int]string{39: "00"}}
	w := NewWorker(mux, safRepo, nil, isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)
	require.NoError(t, w.deliverOnce(ctx))

	require.Equal(t, []string{"0420"}, mux.sentMTIs)
	pending, deadCount, err := safRepo.ListPending(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.Zero(t, deadCount)
}
