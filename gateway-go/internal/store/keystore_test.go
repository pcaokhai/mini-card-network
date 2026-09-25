package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKeyStoreRepository_insertThenActivateRetiresPrevious__MCN_501_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewKeyStoreRepository(pool)
	ctx := context.Background()

	firstID, err := repo.Insert(ctx, KeyRow{KeyType: typeZPK, OwnerRef: "gw-link-01", KeyUnderLMKHex: "aa", KCV: "AABBCC"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, firstID))

	secondID, err := repo.Insert(ctx, KeyRow{KeyType: typeZPK, OwnerRef: "gw-link-01", KeyUnderLMKHex: "bb", KCV: "DDEEFF"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, secondID))

	rows, err := repo.List(ctx)
	require.NoError(t, err)
	statuses := map[int64]string{}
	for _, r := range rows {
		statuses[r.ID] = r.Status
	}
	require.Equal(t, "RETIRED", statuses[firstID])
	require.Equal(t, "ACTIVE", statuses[secondID])
}

func TestKeyStoreRepository_findRecentlyRetiredWithinWindow__MCN_504_AC2(t *testing.T) {
	pool := newTestPool(t)
	repo := NewKeyStoreRepository(pool)
	ctx := context.Background()

	firstID, err := repo.Insert(ctx, KeyRow{KeyType: typeZAK, OwnerRef: "gw-link-01", KeyUnderLMKHex: "aa", KCV: "AAAAAA"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, firstID))

	secondID, err := repo.Insert(ctx, KeyRow{KeyType: typeZAK, OwnerRef: "gw-link-01", KeyUnderLMKHex: "bb", KCV: "BBBBBB"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, secondID)) // retires firstID

	recent, err := repo.FindRecentlyRetired(ctx, typeZAK, "gw-link-01", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, recent)
	require.Equal(t, firstID, recent.ID)

	stale, err := repo.FindRecentlyRetired(ctx, typeZAK, "gw-link-01", 0)
	require.NoError(t, err)
	require.Nil(t, stale)
}

func TestKeyStore_ensureActiveInsertsOnlyWhenNoneActive__MCN_002(t *testing.T) {
	repo := NewKeyStoreRepository(newTestPool(t))
	ctx := context.Background()

	inserted, err := repo.EnsureActive(ctx, typeZAK, "AAAAAAAA", "ABC123")
	require.NoError(t, err)
	require.True(t, inserted)

	inserted, err = repo.EnsureActive(ctx, typeZAK, "BBBBBBBB", "DEF456")
	require.NoError(t, err)
	require.False(t, inserted, "an ACTIVE ZAK already exists")

	rows, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "ACTIVE", rows[0].Status)
	require.Equal(t, "ABC123", rows[0].KCV)
	require.Equal(t, "AAAAAAAA", rows[0].KeyUnderLMKHex)
	require.NotNil(t, rows[0].ActivatedAt)

	active, err := repo.ActiveKCV(ctx, typeZAK)
	require.NoError(t, err)
	require.Equal(t, "ABC123", active)
}

func TestKeyStoreRepository_listCurrentHidesRetiredAndRetirePendingOrphans__SEC_G8(t *testing.T) {
	pool := newTestPool(t)
	repo := NewKeyStoreRepository(pool)
	ctx := context.Background()

	oldID, err := repo.Insert(ctx, KeyRow{KeyType: typeZPK, KeyUnderLMKHex: "aa", KCV: "AAAAAA"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, oldID))
	newID, err := repo.Insert(ctx, KeyRow{KeyType: typeZPK, KeyUnderLMKHex: "bb", KCV: "BBBBBB"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, newID))
	pendingID, err := repo.Insert(ctx, KeyRow{KeyType: typeZAK, KeyUnderLMKHex: "cc", KCV: "CCCCCC"})
	require.NoError(t, err)

	current, err := repo.ListCurrent(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{newID, pendingID}, ids(current))

	require.NoError(t, repo.RetirePending(ctx, pendingID))
	require.NoError(t, repo.RetirePending(ctx, newID), "an ACTIVE key is never retired this way")
	current, err = repo.ListCurrent(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{newID}, ids(current))

	strayID, err := repo.Insert(ctx, KeyRow{KeyType: typeZAK, KeyUnderLMKHex: "dd", KCV: "DDDDDD"})
	require.NoError(t, err)
	n, err := repo.RetireAllPending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	current, err = repo.ListCurrent(ctx)
	require.NoError(t, err)
	require.NotContains(t, ids(current), strayID)
}

const (
	typeZPK = "ZPK"
	typeZAK = "ZAK"
)

func ids(rows []KeyRow) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}
