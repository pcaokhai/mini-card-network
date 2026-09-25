package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalRepository_merchantForSeededTerminal__MCN_002(t *testing.T) {
	repo := NewTerminalRepository(newTestPool(t))

	m, err := repo.Merchant(context.Background(), "00000042")
	require.NoError(t, err)
	require.Equal(t, "GOCPHO000000001", m.MID)
	require.NotEmpty(t, m.Name)
}

func TestTerminalRepository_unknownTerminal__MCN_002(t *testing.T) {
	repo := NewTerminalRepository(newTestPool(t))

	_, err := repo.Merchant(context.Background(), "99999999")
	require.ErrorIs(t, err, ErrUnknownTerminal)
}
