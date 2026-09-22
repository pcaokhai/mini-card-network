package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTranLogRepository_insertUpdateAndGet__MCN_303(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	row := TranLogRow{RRN: "626514000123", Type: "PURCHASE", Status: "CREATED", Amount: 10000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: "GOCPHO000000001"}
	id, err := repo.Insert(ctx, row)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateStatus(ctx, id, "APPROVED"))
	require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))

	got, err := repo.Get(ctx, "626514000123")
	require.NoError(t, err)
	require.Equal(t, "APPROVED", got.Status)
}

func TestIdempotencyRepository_storeAndReplay__MCN_303_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewIdempotencyRepository(pool)
	ctx := context.Background()

	require.NoError(t, repo.Store(ctx, "key-1", "purchases", "hash-abc", 201, []byte(`{"rrn":"x"}`)))
	stored, err := repo.Find(ctx, "key-1", "purchases")
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, 201, stored.Status)
}
