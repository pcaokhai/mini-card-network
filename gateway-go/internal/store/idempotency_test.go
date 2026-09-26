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

	stored, err := repo.Reserve(ctx, "key-1", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.Nil(t, stored, "a fresh key is reserved for this request")

	_, err = repo.Reserve(ctx, "key-1", testIdemRoute, testIdemHash)
	require.ErrorIs(t, err, ErrIdempotencyInProgress, "the first request hasn't finished")

	require.NoError(t, repo.Store(ctx, "key-1", testIdemRoute, testIdemHash, 201, []byte(`{"rrn":"x"}`)))

	stored, err = repo.Reserve(ctx, "key-1", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, 201, stored.Status)
	require.JSONEq(t, `{"rrn":"x"}`, string(stored.Body))

	_, err = repo.Reserve(ctx, "key-1", testIdemRoute, "hash-other")
	require.ErrorIs(t, err, ErrIdempotencyKeyMismatch)
}

func TestIdempotencyRepository_aRecordExpiresAfter24Hours__POS_G3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewIdempotencyRepository(pool)
	ctx := context.Background()
	require.NoError(t, repo.Store(ctx, "key-old", testIdemRoute, testIdemHash, 201, []byte(`{}`)))
	_, err := pool.Exec(ctx, `UPDATE idempotency_record SET created_at = now() - interval '25 hours'`)
	require.NoError(t, err)

	stored, err := repo.Reserve(ctx, "key-old", testIdemRoute, "hash-new")

	require.NoError(t, err)
	require.Nil(t, stored, "an expired key is reserved afresh, whatever it held")
}

func TestIdempotencyRepository_releaseFreesOnlyAPendingReservation__POS_G3(t *testing.T) {
	repo := NewIdempotencyRepository(newTestPool(t))
	ctx := context.Background()
	_, err := repo.Reserve(ctx, "key-r", testIdemRoute, testIdemHash)
	require.NoError(t, err)

	require.NoError(t, repo.Release(ctx, "key-r", testIdemRoute))
	stored, err := repo.Reserve(ctx, "key-r", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.Nil(t, stored)

	require.NoError(t, repo.Store(ctx, "key-r", testIdemRoute, testIdemHash, 201, []byte(`{}`)))
	require.NoError(t, repo.Release(ctx, "key-r", testIdemRoute))
	stored, err = repo.Reserve(ctx, "key-r", testIdemRoute, testIdemHash)
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
			stored, err := repo.Reserve(ctx, "key-race", testIdemRoute, testIdemHash)
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
	_, err := repo.Reserve(ctx, "key-p", testIdemRoute, testIdemHash)
	require.NoError(t, err)
	require.NoError(t, repo.AttachRRN(ctx, "key-p", testIdemRoute, "626514000801"))

	_, err = repo.Reserve(ctx, "key-p", testIdemRoute, testIdemHash)

	require.ErrorIs(t, err, ErrIdempotencyInProgress)
	var pending *InProgressError
	require.ErrorAs(t, err, &pending)
	require.Equal(t, "626514000801", pending.RRN, "a retry can find what the first request sent")
	require.WithinDuration(t, time.Now(), pending.ReservedAt, time.Minute)
}
