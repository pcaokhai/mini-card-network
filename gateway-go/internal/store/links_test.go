package store

import (
	"context"
	"strconv"
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
	events, _, err := repo.ListEvents(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestLinkRepository_appendEventReturnsTheStoredRowWithCode__NET_G4_G15(t *testing.T) {
	repo := NewLinkRepository(newTestPool(t))
	ctx := context.Background()

	stored, err := repo.AppendEvent(ctx, NetworkEvent{Code: "SIGNED_OFF", Severity: "INFO", EasyText: "Signed off", TechnicalText: "0800 DE70=002"})
	require.NoError(t, err)
	require.NotZero(t, stored.ID)
	require.False(t, stored.OccurredAt.IsZero())
	require.Equal(t, "SIGNED_OFF", stored.Code)

	events, _, err := repo.ListEvents(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, stored.ID, events[0].ID)
	require.Equal(t, "SIGNED_OFF", events[0].Code)
}

func TestLinkRepository_listEventsPagesByKeyset__NET_G12(t *testing.T) {
	repo := NewLinkRepository(newTestPool(t))
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := repo.AppendEvent(ctx, NetworkEvent{Code: "ECHO_OK", Severity: "INFO", EasyText: "e", TechnicalText: strconv.Itoa(i)})
		require.NoError(t, err)
	}

	page1, next, err := repo.ListEvents(ctx, 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.Equal(t, "4", page1[0].TechnicalText)
	require.NotEmpty(t, next)

	page2, next, err := repo.ListEvents(ctx, 2, next)
	require.NoError(t, err)
	require.Equal(t, []string{"2", "1"}, []string{page2[0].TechnicalText, page2[1].TechnicalText})

	page3, next, err := repo.ListEvents(ctx, 2, next)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	require.Empty(t, next)

	_, _, err = repo.ListEvents(ctx, 2, "not-a-cursor")
	require.ErrorIs(t, err, ErrInvalidCursor)
}
