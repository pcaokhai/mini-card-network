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

func TestTerminalRepository_upsertFromFixtureAddsAndRenames__MCN_002(t *testing.T) {
	repo := NewTerminalRepository(newTestPool(t))
	ctx := context.Background()
	fixture := []FixtureTerminal{
		{TerminalID: "00000042", MerchantID: "GOCPHO000000001", MerchantName: "Cà phê Góc Phố", MCC: "5814"},
		{TerminalID: "00000047", MerchantID: "BANHMA000000001", MerchantName: "Tiệm bánh Mây", MCC: "5462"},
	}

	require.NoError(t, repo.UpsertFromFixture(ctx, fixture))
	require.NoError(t, repo.UpsertFromFixture(ctx, fixture), "idempotent")

	renamed, err := repo.Merchant(ctx, "00000042")
	require.NoError(t, err)
	require.Equal(t, "Cà phê Góc Phố", renamed.Name, "the migration's ASCII name is corrected")
	added, err := repo.Merchant(ctx, "00000047")
	require.NoError(t, err)
	require.Equal(t, Merchant{MID: "BANHMA000000001", Name: "Tiệm bánh Mây"}, added)
}

func TestTerminalRepository_listJoinsMerchants__NET_G13(t *testing.T) {
	repo := NewTerminalRepository(newTestPool(t))

	terminals, err := repo.List(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, terminals)
	require.Equal(t, FixtureTerminal{TerminalID: "00000042", MerchantID: "GOCPHO000000001", MerchantName: "Ca phe Goc Pho", MCC: "5814"}, terminals[0])
}
