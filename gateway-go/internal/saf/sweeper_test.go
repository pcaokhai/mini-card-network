package saf

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeOrphans struct {
	rows     []store.TranLogRow
	timedOut []int64
	findErr  error
}

func (f *fakeOrphans) FindOrphans(context.Context, time.Time, int) ([]store.TranLogRow, error) {
	return f.rows, f.findErr
}

func (f *fakeOrphans) MarkTimedOut(_ context.Context, id int64) (bool, error) {
	f.timedOut = append(f.timedOut, id)
	return true, nil
}

type recordingQueuer struct {
	reversals []store.TranLogRow
	reasons   []string
	advices   []map[int]string
	queueErr  error
}

func (q *recordingQueuer) Queue(_ context.Context, txn store.TranLogRow, reason string) error {
	q.reversals, q.reasons = append(q.reversals, txn), append(q.reasons, reason)
	return q.queueErr
}

func (q *recordingQueuer) QueueAdvice(_ context.Context, _ int64, _ string, sent map[int]string) error {
	q.advices = append(q.advices, sent)
	return nil
}

func orphan(id int64, tranType, status string) store.TranLogRow {
	sent := time.Date(2026, 9, 25, 7, 32, 44, 0, time.UTC)
	return store.TranLogRow{ID: id, RRN: fmt.Sprintf("6265140010%02d", id), Type: tranType, Status: status, Amount: 5000, Currency: "704",
		TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000100", ProcessingCode: "000000", SentAt: &sent, CardToken: testCardToken}
}

func TestSweeper_followsUpEveryOrphanByType__POS_G16(t *testing.T) {
	completion := orphan(3, "COMPLETION", "SENT")
	completion.OriginalRRN = "626514000300"
	orphans := &fakeOrphans{rows: []store.TranLogRow{
		orphan(1, "PURCHASE", "SENT"), orphan(2, "PREAUTH", "TIMED_OUT"), completion, orphan(4, "BALANCE", "SENT"),
	}}
	queuer := &recordingQueuer{}

	require.NoError(t, NewSweeper(orphans, queuer, time.Minute, time.Second).SweepOnce(context.Background()))

	require.Equal(t, []int64{1, 3, 4}, orphans.timedOut, "every SENT orphan becomes TIMED_OUT first")
	require.Equal(t, []string{"68", "68"}, queuer.reasons, "the purchase and the pre-auth are reversed")
	for _, r := range queuer.reversals {
		require.Equal(t, "TIMED_OUT", r.Status, "queued from the state the row now holds")
	}
	require.Len(t, queuer.advices, 1, "the completion's 0220 is repeated, never reversed")
	advice := queuer.advices[0]
	require.Equal(t, "626514000300", advice[37], "the 0220 names the pre-auth")
	require.Equal(t, "000100", advice[11])
	require.Equal(t, "0925073244", advice[7], "the same message as sent, so the issuer dedupes the 0221")
	require.Equal(t, "00000042", advice[41])
	require.Equal(t, testMerchantID, advice[42])
	require.Equal(t, "000000005000", advice[4])
}

func TestSweeper_oneFailureDoesNotStopTheSweep__POS_G16(t *testing.T) {
	orphans := &fakeOrphans{rows: []store.TranLogRow{orphan(1, "PURCHASE", "SENT"), orphan(2, "REFUND", "SENT")}}
	queuer := &recordingQueuer{queueErr: errors.New("db down")}

	err := NewSweeper(orphans, queuer, time.Minute, time.Second).SweepOnce(context.Background())

	require.Error(t, err)
	require.Len(t, queuer.reasons, 2, "both orphans were attempted")
}

func TestSweeper_runStopsOnCancel__POS_G16(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- NewSweeper(&fakeOrphans{}, &recordingQueuer{}, time.Minute, time.Millisecond).Run(ctx) }()
	cancel()
	require.NoError(t, <-done)
}
