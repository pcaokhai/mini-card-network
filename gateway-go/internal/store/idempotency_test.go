package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testIdemRoute = "purchases"
	testIdemHash  = "hash-abc"
)

func TestIdempotencyRepository_reserveReplaysOnlyTheSameRequest__POS_G3(t *testing.T) {
	repo := NewIdempotencyRepository(newTestPool(t))
	ctx := context.Background()

	stored, token, err := repo.Reserve(ctx, "key-1", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.Nil(t, stored, "a fresh key is reserved for this request")
	require.NotEmpty(t, token, "the reservation is fenced by its token")

	_, _, err = repo.Reserve(ctx, "key-1", testIdemRoute, testIdemHash)
	require.ErrorIs(t, err, ErrIdempotencyInProgress, "the first request hasn't finished")

	require.NoError(t, repo.Store(ctx, "key-1", testIdemRoute, testIdemHash, 201, []byte(`{"rrn":"x"}`)))

	stored, token, err = repo.Reserve(ctx, "key-1", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Empty(t, token, "a replay holds no reservation")
	require.Equal(t, 201, stored.Status)
	require.JSONEq(t, `{"rrn":"x"}`, string(stored.Body))

	_, _, err = repo.Reserve(ctx, "key-1", testIdemRoute, "hash-other")
	require.ErrorIs(t, err, ErrIdempotencyKeyMismatch)
}

func TestIdempotencyRepository_aRecordExpiresAfter24Hours__POS_G3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewIdempotencyRepository(pool)
	ctx := context.Background()
	require.NoError(t, repo.Store(ctx, "key-old", testIdemRoute, testIdemHash, 201, []byte(`{}`)))
	_, err := pool.Exec(ctx, `UPDATE idempotency_record SET created_at = now() - interval '25 hours'`)
	require.NoError(t, err)

	stored, token, err := repo.Reserve(ctx, "key-old", testIdemRoute, "hash-new")

	require.NoError(t, err)
	require.Nil(t, stored, "an expired key is reserved afresh, whatever it held")
	require.NotEmpty(t, token)
}

func TestIdempotencyRepository_releaseFreesOnlyAPendingReservation__POS_G3(t *testing.T) {
	repo := NewIdempotencyRepository(newTestPool(t))
	ctx := context.Background()
	_, token, err := repo.Reserve(ctx, "key-r", testIdemRoute, testIdemHash)
	require.NoError(t, err)

	require.NoError(t, repo.Release(ctx, "key-r", testIdemRoute, token))
	stored, token, err := repo.Reserve(ctx, "key-r", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.Nil(t, stored)

	require.NoError(t, repo.Store(ctx, "key-r", testIdemRoute, testIdemHash, 201, []byte(`{}`)))
	require.NoError(t, repo.Release(ctx, "key-r", testIdemRoute, token))
	stored, _, err = repo.Reserve(ctx, "key-r", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.NotNil(t, stored, "a finished response is never released")
}

func TestIdempotencyRepository_concurrentRequestsReserveOnce__POS_G3(t *testing.T) {
	repo := NewIdempotencyRepository(newTestPool(t))
	ctx := context.Background()
	var reserved atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stored, _, err := repo.Reserve(ctx, "key-race", testIdemRoute, testIdemHash)
			if err == nil && stored == nil {
				reserved.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), reserved.Load())
}

func TestIdempotencyRepository_aPendingKeyReportsItsRRNAndAge__POS_G3(t *testing.T) {
	repo := NewIdempotencyRepository(newTestPool(t))
	ctx := context.Background()
	_, token, err := repo.Reserve(ctx, "key-p", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.NoError(t, repo.AttachRRN(ctx, "key-p", testIdemRoute, token, "626514000801"))

	_, _, err = repo.Reserve(ctx, "key-p", testIdemRoute, testIdemHash)

	require.ErrorIs(t, err, ErrIdempotencyInProgress)
	var pending *InProgressError
	require.ErrorAs(t, err, &pending)
	require.Equal(t, "626514000801", pending.RRN, "a retry can find what the first request sent")
	require.WithinDuration(t, time.Now(), pending.ReservedAt, time.Minute)
}

func ageReservations(ctx context.Context, t *testing.T, pool *Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `UPDATE idempotency_record SET created_at = now() - interval '5 minutes'`)
	require.NoError(t, err)
}

func TestIdempotencyRepository_reclaimsAStaleKeyThatNeverSent__POS_G3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewIdempotencyRepository(pool)
	ctx := context.Background()
	_, _, err := repo.Reserve(ctx, "key-s", testIdemRoute, testIdemHash)
	require.NoError(t, err)

	token, err := repo.Reclaim(ctx, "key-s", testIdemRoute, testIdemHash, time.Minute)
	require.NoError(t, err)
	require.Empty(t, token, "a fresh reservation may still be about to send")

	ageReservations(ctx, t, pool)
	token, err = repo.Reclaim(ctx, "key-s", testIdemRoute, testIdemHash, time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	again, err := repo.Reclaim(ctx, "key-s", testIdemRoute, testIdemHash, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again, "reclaiming refreshes the reservation, so a concurrent retry can't reclaim it too")

	ageReservations(ctx, t, pool)
	require.NoError(t, repo.AttachRRN(ctx, "key-s", testIdemRoute, token, "626514000001"))
	again, err = repo.Reclaim(ctx, "key-s", testIdemRoute, testIdemHash, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again, "a key that sent is answered from tran_log, never re-sent")
}

// A holder that was only slow, not dead, finds its reservation reclaimed: it must abort before
// sending, so one key never sends twice (#117 re-review blocker).
func TestIdempotencyRepository_aReclaimedHolderCantAttachAnRRN__POS_G3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewIdempotencyRepository(pool)
	ctx := context.Background()
	_, stale, err := repo.Reserve(ctx, "key-slow", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	ageReservations(ctx, t, pool)
	fresh, err := repo.Reclaim(ctx, "key-slow", testIdemRoute, testIdemHash, time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, fresh)

	require.ErrorIs(t, repo.AttachRRN(ctx, "key-slow", testIdemRoute, stale, "626514000901"), ErrReservationLost)
	require.NoError(t, repo.AttachRRN(ctx, "key-slow", testIdemRoute, fresh, "626514000902"))

	_, _, err = repo.Reserve(ctx, "key-slow", testIdemRoute, testIdemHash)
	var pending *InProgressError
	require.ErrorAs(t, err, &pending)
	require.Equal(t, "626514000902", pending.RRN, "only the new holder's RRN is recorded")
}

// A slow holder that then fails before sending releases only its own reservation, never the one
// that reclaimed the key, so a third retry can't reserve and send again.
func TestIdempotencyRepository_aReclaimedHolderCantReleaseTheNewReservation__POS_G3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewIdempotencyRepository(pool)
	ctx := context.Background()
	_, stale, err := repo.Reserve(ctx, "key-slow", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	ageReservations(ctx, t, pool)
	fresh, err := repo.Reclaim(ctx, "key-slow", testIdemRoute, testIdemHash, time.Minute)
	require.NoError(t, err)

	require.NoError(t, repo.Release(ctx, "key-slow", testIdemRoute, stale))

	_, _, err = repo.Reserve(ctx, "key-slow", testIdemRoute, testIdemHash)
	require.ErrorIs(t, err, ErrIdempotencyInProgress, "the new holder's reservation is intact")
	require.NoError(t, repo.Release(ctx, "key-slow", testIdemRoute, fresh))
	stored, token, err := repo.Reserve(ctx, "key-slow", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.Nil(t, stored)
	require.NotEmpty(t, token, "its own holder can release it")
}
