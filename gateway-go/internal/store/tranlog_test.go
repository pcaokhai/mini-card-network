package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testMerchantID   = "GOCPHO000000001"
	testMaskedPAN    = "970436******4417"
	statusApproved   = "APPROVED"
	tranTypePurchase = "PURCHASE"
)

func TestTranLogRepository_insertUpdateAndGet__MCN_303(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	row := TranLogRow{RRN: "626514000123", Type: tranTypePurchase, Status: "CREATED", Amount: 10000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID}
	id, err := repo.Insert(ctx, row)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateStatus(ctx, id, statusApproved, "00", "123456"))
	require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", statusApproved))

	got, err := repo.Get(ctx, "626514000123")
	require.NoError(t, err)
	require.Equal(t, statusApproved, got.Status)
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

func TestTranLogRepository_updateLateResponseSetsCodeAndTimestamp__MCN_403_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	_, err := repo.Insert(ctx, TranLogRow{RRN: "626514000999", Type: tranTypePurchase, Status: "TIMED_OUT", Amount: 5000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID})
	require.NoError(t, err)

	require.NoError(t, repo.UpdateLateResponse(ctx, "626514000999", "00"))

	got, err := repo.Get(ctx, "626514000999")
	require.NoError(t, err)
	require.Equal(t, "TIMED_OUT", got.Status) // status unchanged - only the late-response columns move
	require.Equal(t, "00", got.LateResponseCode)
	require.NotNil(t, got.LateResponseAt)
}

func TestTranLogRepository_listFiltersAndPaginates__MCN_304_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		row := TranLogRow{RRN: fmt.Sprintf("rrn-%d", i), Status: statusApproved, Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase}
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
	_, err := repo.Insert(ctx, TranLogRow{RRN: "a", Status: statusApproved, Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase})
	require.NoError(t, err)
	_, err = repo.Insert(ctx, TranLogRow{RRN: "b", Status: "DECLINED", Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase})
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
	_, err := repo.Insert(ctx, TranLogRow{RRN: "findme", Status: statusApproved, Amount: 500, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase})
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
	id, err := repo.Insert(ctx, TranLogRow{RRN: "hist", Status: "CREATED", Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase})
	require.NoError(t, err)
	require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
	require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", statusApproved))

	history, err := repo.ListStateHistory(ctx, id)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "SENT", history[0].ToStatus)
	require.Equal(t, statusApproved, history[1].ToStatus)
}

func TestTranLogRepository_roundTripsTheFieldsAReversalNeeds__MCN_401(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	sentAt := time.Date(2026, 9, 21, 7, 32, 44, 0, time.UTC)

	_, err := repo.Insert(ctx, TranLogRow{
		RRN: "626514000701", Type: tranTypePurchase, Status: "CREATED", Amount: 600000, Currency: "704",
		MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000124",
		ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: "tok_normal",
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, "626514000701")
	require.NoError(t, err)
	require.Equal(t, "000124", got.NetworkSTAN)
	require.Equal(t, "000000", got.ProcessingCode)
	require.Equal(t, "051", got.POSEntryMode)
	require.Equal(t, "tok_normal", got.CardToken)
	require.NotNil(t, got.SentAt)
	require.True(t, got.SentAt.Equal(sentAt))

	page, _, err := repo.List(ctx, TransactionFilter{})
	require.NoError(t, err)
	require.Equal(t, "000124", page[0].NetworkSTAN, "list rows carry the STAN too")
}

func TestTranLogRepository_lastSTANInTheHoursRRNPrefix__MCN_203(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	for _, rrn := range []string{"626805000007", "626805000248", "626804000999", "626806000001"} {
		_, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: tranTypePurchase, Status: "APPROVED", Amount: 1000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID})
		require.NoError(t, err)
	}

	last, err := repo.LastSTANInRRNPrefix(ctx, "626805")
	require.NoError(t, err)
	require.Equal(t, int64(248), last)

	none, err := repo.LastSTANInRRNPrefix(ctx, "626807")
	require.NoError(t, err)
	require.Zero(t, none)
}
