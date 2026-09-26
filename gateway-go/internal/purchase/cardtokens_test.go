package purchase

import (
	"encoding/json"
	"os"
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

// Every seed card carries its issuer cardRef, generated from contracts/fixtures/cards.json, so no
// hand-kept map can drift from it (CHA-G14).
func TestCardTokens_seedsCarryTheirCardRefFromTheFixtures__CHA_G14(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/fixtures/cards.json")
	require.NoError(t, err)
	var fixture struct {
		Cards []struct{ CardToken, CardRef string }
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	want := map[string]string{}
	for _, c := range fixture.Cards {
		want[c.CardToken] = c.CardRef
	}

	got := map[string]string{}
	for _, seed := range DefaultCardTokens().Seeds() {
		got[seed.CardToken] = seed.CardRef
	}

	require.Equal(t, want, got)
}
