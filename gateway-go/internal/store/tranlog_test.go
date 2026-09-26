package store

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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

func TestTranLogRepository_backdateMovesTheRowAndItsHistoryTogether__MCN_002(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()

	id, err := repo.Insert(ctx, TranLogRow{RRN: "626514000601", Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: "970436******4417", TerminalID: "00000042", MerchantID: testMerchantID})
	require.NoError(t, err)
	require.NoError(t, repo.RecordStateTransition(ctx, id, "CREATED", "SENT"))
	require.NoError(t, repo.RecordStateTransition(ctx, id, "SENT", "APPROVED"))
	before, err := repo.ListStateHistory(ctx, id)
	require.NoError(t, err)
	target := time.Now().UTC().Add(-26 * time.Hour).Truncate(time.Microsecond)

	require.NoError(t, repo.Backdate(ctx, "626514000601", target))

	row, err := repo.Get(ctx, "626514000601")
	require.NoError(t, err)
	require.True(t, row.CreatedAt.Equal(target), "created_at %s, want %s", row.CreatedAt, target)
	after, err := repo.ListStateHistory(ctx, id)
	require.NoError(t, err)
	require.Len(t, after, len(before))
	shift := after[0].At.Sub(before[0].At)
	require.InDelta(t, -26*time.Hour, shift, float64(time.Minute))
	for i := range after {
		require.Equal(t, before[i].At.Add(shift), after[i].At, "history row %d keeps its spacing", i)
	}
}

// The seed backdates whole transactions: every timestamp the journey reads moves by the same shift,
// or a reversal's steps land hundreds of seconds after its purchase.
func TestTranLogRepository_backdateMovesEveryJourneyTimestamp__MCN_002(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	safRepo := NewSafRepository(pool)
	ctx := context.Background()
	sentAt := time.Now().UTC().Truncate(time.Microsecond)
	id, err := repo.Insert(ctx, TranLogRow{RRN: "626514000602", Type: tranTypePurchase, Status: "CREATED", Amount: 1000, Currency: "704", MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID, SentAt: &sentAt})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateStatus(ctx, id, "APPROVED", "00", "A1"))
	safID, err := safRepo.Enqueue(ctx, id, "0420", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, safRepo.MarkAcked(ctx, safID))
	before, err := repo.Get(ctx, "626514000602")
	require.NoError(t, err)
	revBefore, err := safRepo.FindReversal(ctx, id)
	require.NoError(t, err)

	require.NoError(t, repo.Backdate(ctx, "626514000602", before.CreatedAt.Add(-26*time.Hour)))

	after, err := repo.Get(ctx, "626514000602")
	require.NoError(t, err)
	revAfter, err := safRepo.FindReversal(ctx, id)
	require.NoError(t, err)
	shift := -26 * time.Hour
	require.True(t, after.SentAt.Equal(before.SentAt.Add(shift)), "sent_at")
	require.True(t, after.RespondedAt.Equal(before.RespondedAt.Add(shift)), "responded_at")
	require.True(t, revAfter.CreatedAt.Equal(revBefore.CreatedAt.Add(shift)), "saf created_at")
	require.True(t, revAfter.AckedAt.Equal(revBefore.AckedAt.Add(shift)), "saf acked_at")
}

func TestTranLogRepository_backdateUnknownRRN__MCN_002(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))

	err := repo.Backdate(context.Background(), "000000000000", time.Now())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestTranLogRepository_roundTripsTheFieldsAReversalNeeds__MCN_401(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	sentAt := time.Date(2026, 9, 21, 7, 32, 44, 0, time.UTC)

	_, err := repo.Insert(ctx, TranLogRow{
		RRN: "626514000701", Type: tranTypePurchase, Status: "CREATED", Amount: 600000, Currency: "704",
		MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID, NetworkSTAN: "000124",
		ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: "tok_normal", MTI: "0100",
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, "626514000701")
	require.NoError(t, err)
	require.Equal(t, "0100", got.MTI, "DE 90 of the 0420 names the original MTI")
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

func TestTranLogRepository_getReadsRespondedAt__MCN_304(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	id, err := repo.Insert(ctx, TranLogRow{RRN: "answered", Status: "SENT", Amount: 500, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase})
	require.NoError(t, err)

	before, err := repo.Get(ctx, "answered")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateStatus(ctx, id, statusApproved, "00", "A00001"))
	after, err := repo.Get(ctx, "answered")
	require.NoError(t, err)

	require.Nil(t, before.RespondedAt)
	require.NotNil(t, after.RespondedAt)
}

func TestTranLogRepository_roundTripsTheDetailFields__JRN_G3(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	id, err := repo.Insert(ctx, TranLogRow{
		RRN: "626514000702", Type: "COMPLETION", Status: statusApproved, Amount: 10000, Currency: "704",
		MaskedPAN: testMaskedPAN, TerminalID: "00000042", MerchantID: testMerchantID,
		OriginalRRN: "626514000700", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	require.NoError(t, err)
	approved := int64(7500)
	require.NoError(t, repo.UpdateAmounts(ctx, id, &approved, &Money{Amount: 42000, Currency: "704"}))

	got, err := repo.Get(ctx, "626514000702")

	require.NoError(t, err)
	require.Equal(t, "626514000700", got.OriginalRRN)
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", got.TraceID)
	require.Equal(t, &approved, got.ApprovedAmount)
	require.Equal(t, &Money{Amount: 42000, Currency: "704"}, got.Balance)
}

func TestTranLogRepository_listFiltersByReversalReason__JRN_G7(t *testing.T) {
	pool := newTestPool(t)
	repo := NewTranLogRepository(pool)
	ctx := context.Background()
	for rrn, reason := range map[string]string{"r68": "68", "r17": "17", "r06": "06", "r99": "99", "none": ""} {
		id, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Status: "REVERSED", Amount: 1, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID, Type: tranTypePurchase})
		require.NoError(t, err)
		if reason != "" {
			_, err = pool.Exec(ctx, `UPDATE tran_log SET reversal_reason = $2 WHERE id = $1`, id, reason)
			require.NoError(t, err)
		}
	}
	for filter, want := range map[string]string{
		ReversalTimeout: "r68", ReversalCustomerCancellation: "r17", ReversalMACFailure: "r06", ReversalSendFailure: "r99",
	} {
		page, _, err := repo.List(ctx, TransactionFilter{ReversalReason: &filter})
		require.NoError(t, err)
		require.Len(t, page, 1, filter)
		require.Equal(t, want, page[0].RRN, filter)
		require.Equal(t, filter, ReversalReasonOf(page[0].ReversalReasonCode))
	}
	require.Empty(t, ReversalReasonOf(""))
}

func TestTranLogRepository_listRejectsAMalformedCursor__JRN_G6(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))

	_, _, err := repo.List(context.Background(), TransactionFilter{Cursor: "zzz"})

	require.ErrorIs(t, err, ErrInvalidCursor)
}

func insertPreAuth(ctx context.Context, t *testing.T, repo *TranLogRepository, rrn, status string) {
	t.Helper()
	_, err := repo.Insert(ctx, TranLogRow{RRN: rrn, Type: "PREAUTH", Status: status, Amount: 10000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID})
	require.NoError(t, err)
}

func completionOf(rrn string) TranLogRow {
	return TranLogRow{RRN: rrn, Type: "COMPLETION", Status: statusApproved, Amount: 5000, Currency: "704", TerminalID: "00000042", MerchantID: testMerchantID}
}

func TestTranLogRepository_aPreAuthIsCompletedOnce__S2(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	insertPreAuth(ctx, t, repo, "626514000901", statusApproved)
	insertPreAuth(ctx, t, repo, "626514000902", "DECLINED")

	_, err := repo.InsertCompletion(ctx, completionOf("626514000911"), "626514000901")
	require.NoError(t, err)
	_, err = repo.InsertCompletion(ctx, completionOf("626514000912"), "626514000901")
	require.ErrorIs(t, err, ErrNotCompletable, "a second completion of the same pre-auth")
	_, err = repo.InsertCompletion(ctx, completionOf("626514000913"), "626514000902")
	require.ErrorIs(t, err, ErrNotCompletable, "a declined pre-auth holds nothing")

	preAuth, err := repo.Get(ctx, "626514000901")
	require.NoError(t, err)
	require.Equal(t, "626514000911", preAuth.CompletedBy)
	_, err = repo.Get(ctx, "626514000912")
	require.ErrorIs(t, err, ErrNotFound, "the refused completion leaves no row")
}

func TestTranLogRepository_concurrentCompletionsClaimThePreAuthOnce__S2(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	insertPreAuth(ctx, t, repo, "626514000921", statusApproved)
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.InsertCompletion(ctx, completionOf(fmt.Sprintf("62651400093%d", i)), "626514000921"); err == nil {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), won.Load())
}

func TestTranLogRepository_releasingAClaimLetsThePreAuthBeCompletedAgain__S2(t *testing.T) {
	repo := NewTranLogRepository(newTestPool(t))
	ctx := context.Background()
	insertPreAuth(ctx, t, repo, "626514000961", statusApproved)
	_, err := repo.InsertCompletion(ctx, completionOf("626514000962"), "626514000961")
	require.NoError(t, err)

	require.NoError(t, repo.ReleaseCompletion(ctx, "626514000961", "626514000999"), "another completion's release is a no-op")
	_, err = repo.InsertCompletion(ctx, completionOf("626514000963"), "626514000961")
	require.ErrorIs(t, err, ErrNotCompletable)

	require.NoError(t, repo.ReleaseCompletion(ctx, "626514000961", "626514000962"))
	_, err = repo.InsertCompletion(ctx, completionOf("626514000964"), "626514000961")
	require.NoError(t, err)
}
