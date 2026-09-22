package store

import (
	"context"
	"fmt"
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

	require.NoError(t, repo.UpdateStatus(ctx, id, "APPROVED", "00", "123456"))
	require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))

	got, err := repo.Get(ctx, "626514000123")
	require.NoError(t, err)
	require.Equal(t, "APPROVED", got.Status)
	require.Equal(t, "00", got.ResponseCode)
	require.Equal(t, "123456", got.AuthCode)
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

func TestTranLogRepository_listFiltersAndPaginates__MCN_304_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		row := TranLogRow{RRN: fmt.Sprintf("rrn-%d", i), Status: "APPROVED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: "GOCPHO000000001", Type: "PURCHASE"}
		_, err := repo.Insert(ctx, row)
		require.NoError(t, err)
	}

	page, cursor, err := repo.List(ctx, TransactionFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.NotEmpty(t, cursor)
	// newest first
	require.Equal(t, "rrn-2", page[0].RRN)
	require.Equal(t, "rrn-1", page[1].RRN)

	nextPage, nextCursor, err := repo.List(ctx, TransactionFilter{Limit: 2, Cursor: cursor})
	require.NoError(t, err)
	require.Len(t, nextPage, 1)
	require.Equal(t, "rrn-0", nextPage[0].RRN)
	require.Empty(t, nextCursor)
}

func TestTranLogRepository_listFiltersByStatus__MCN_304_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	_, err := repo.Insert(ctx, TranLogRow{RRN: "a", Status: "APPROVED", Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", Type: "PURCHASE"})
	require.NoError(t, err)
	_, err = repo.Insert(ctx, TranLogRow{RRN: "b", Status: "DECLINED", Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", Type: "PURCHASE"})
	require.NoError(t, err)

	status := "DECLINED"
	page, _, err := repo.List(ctx, TransactionFilter{Status: &status, Limit: 10})
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "b", page[0].RRN)
}

func TestTranLogRepository_getByRrn__MCN_304_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	_, err := repo.Insert(ctx, TranLogRow{RRN: "findme", Status: "APPROVED", Amount: 500, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", Type: "PURCHASE"})
	require.NoError(t, err)

	got, err := repo.Get(ctx, "findme")
	require.NoError(t, err)
	require.Equal(t, "findme", got.RRN)
	require.Equal(t, "Ca phe Goc Pho", got.MerchantName)
	require.False(t, got.CreatedAt.IsZero())
}

func TestTranLogRepository_listStateHistory__MCN_304_AC2(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	id, err := repo.Insert(ctx, TranLogRow{RRN: "hist", Status: "CREATED", Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001", Type: "PURCHASE"})
	require.NoError(t, err)
	require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
	require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))

	history, err := repo.ListStateHistory(ctx, id)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "SENT", history[0].ToStatus)
	require.Equal(t, "APPROVED", history[1].ToStatus)
}
