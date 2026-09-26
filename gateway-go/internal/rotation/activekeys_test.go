package rotation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeKeyRows struct {
	rows []store.KeyRow
	err  error
}

func (f *fakeKeyRows) List(context.Context) ([]store.KeyRow, error) { return f.rows, f.err }

const (
	zakHex       = "0a0a"
	statusActive = "ACTIVE"
)

func TestActiveKeys_reloadPicksUpTheNewlyActivatedZAK__SEC_G10(t *testing.T) {
	rows := &fakeKeyRows{rows: []store.KeyRow{{KeyType: typeZAK, Status: statusActive, KeyUnderLMKHex: zakHex}}}
	keys := NewActiveKeys(rows, fakeHSM{}) // fakeHSM.Unwrap returns its input
	require.Nil(t, keys.ActiveZAK())

	require.NoError(t, keys.Reload(context.Background(), typeZAK))
	require.Equal(t, []byte{0x0a, 0x0a}, keys.ActiveZAK())

	rows.rows = []store.KeyRow{
		{KeyType: typeZAK, Status: "RETIRED", KeyUnderLMKHex: zakHex},
		{KeyType: typeZAK, Status: statusActive, KeyUnderLMKHex: "0b0b"},
	}
	require.NoError(t, keys.Reload(context.Background(), typeZAK))
	require.Equal(t, []byte{0x0b, 0x0b}, keys.ActiveZAK())
}

func TestActiveKeys_aFailedReloadKeepsTheCurrentKey__SEC_G10(t *testing.T) {
	rows := &fakeKeyRows{rows: []store.KeyRow{{KeyType: typeZAK, Status: statusActive, KeyUnderLMKHex: zakHex}}}
	keys := NewActiveKeys(rows, fakeHSM{})
	require.NoError(t, keys.Reload(context.Background(), typeZAK))

	rows.err = errors.New("db down")
	require.Error(t, keys.Reload(context.Background(), typeZAK))
	require.Equal(t, []byte{0x0a, 0x0a}, keys.ActiveZAK())

	require.NoError(t, keys.Reload(context.Background(), "ZPK"), "only the ZAK is held in the clear")
}
