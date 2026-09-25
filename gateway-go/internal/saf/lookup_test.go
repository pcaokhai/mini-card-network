package saf

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeReversalRows struct {
	row store.SafRow
	err error
}

func (f fakeReversalRows) FindReversal(context.Context, int64) (store.SafRow, error) {
	return f.row, f.err
}

func TestReversalLookup_decodesStoredAdvice__MCN_304(t *testing.T) {
	key := make([]byte, 32)
	stored := advice{CardToken: "tok_visa_ok", Fields: map[int]string{11: "000125", 39: "68", 90: "0200000124"}}
	payload, err := encodePayload(key, stored)
	require.NoError(t, err)
	queued, acked := time.Now().Add(-time.Minute), time.Now()
	lookup := NewReversalLookup(fakeReversalRows{row: store.SafRow{Status: "ACKED", Attempts: 2, Payload: payload, CreatedAt: queued, AckedAt: &acked}}, key)

	rev, err := lookup.Reversal(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, "ACKED", rev.Status)
	require.Equal(t, 2, rev.Attempts)
	require.Equal(t, queued, rev.QueuedAt)
	require.Equal(t, &acked, rev.AckedAt)
	require.Equal(t, stored.Fields, rev.Fields)
}

func TestReversalLookup_noQueuedReversalIsNil__MCN_304(t *testing.T) {
	lookup := NewReversalLookup(fakeReversalRows{err: store.ErrNotFound}, nil)

	rev, err := lookup.Reversal(context.Background(), 1)

	require.NoError(t, err)
	require.Nil(t, rev)
}
