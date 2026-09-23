package rotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mcn/gateway-go/internal/store"
)

func newTestPool(t *testing.T) *store.Pool {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("acquirer"), postgres.WithUsername("acquirer"), postgres.WithPassword("test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, store.Migrate(dsn))
	pool, err := store.Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}
