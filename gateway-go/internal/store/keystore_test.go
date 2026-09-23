package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyStoreRepository_insertThenActivateRetiresPrevious__MCN_501_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewKeyStoreRepository(pool)
	ctx := context.Background()

	firstID, err := repo.Insert(ctx, KeyRow{KeyType: "ZPK", OwnerRef: "gw-link-01", KeyUnderLMKHex: "aa", KCV: "AABBCC"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, firstID))

	secondID, err := repo.Insert(ctx, KeyRow{KeyType: "ZPK", OwnerRef: "gw-link-01", KeyUnderLMKHex: "bb", KCV: "DDEEFF"})
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
