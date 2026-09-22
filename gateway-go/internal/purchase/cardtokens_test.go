package purchase

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCardTokens_resolvesKnownToken__MCN_303(t *testing.T) {
	registry := DefaultCardTokens()

	card, ok := registry.Resolve("tok_normal")
	require.True(t, ok)
	require.Equal(t, "9704360000004417", card.PAN)
	require.Equal(t, "2811", card.ExpiryYYMM)
}

func TestCardTokens_unknownTokenNotFound__MCN_303(t *testing.T) {
	registry := DefaultCardTokens()
	_, ok := registry.Resolve("tok_does_not_exist")
	require.False(t, ok)
}
