package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func newTestPool(t *testing.T) *Pool {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("acquirer"), postgres.WithUsername("acquirer"), postgres.WithPassword("test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, Migrate(dsn))
	pool, err := Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestLinkRepository_upsertAndRead__MCN_202_AC3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewLinkRepository(pool)
	ctx := context.Background()

	require.NoError(t, repo.SetStatus(ctx, "issuer", "SIGNED_ON"))
	link, err := repo.Get(ctx, "issuer")
	require.NoError(t, err)
	require.Equal(t, "SIGNED_ON", link.Status)

	require.NoError(t, repo.RecordEvent(ctx, "INFO", "Link is healthy", "issuer SIGNED_ON"))
	events, err := repo.ListEvents(ctx, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
}
