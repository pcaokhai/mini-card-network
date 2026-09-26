package purchase

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

// fakeFallbackKeys offers several fallback keys, like store.MACFallbackKeys.
type fakeFallbackKeys struct{ rows []store.KeyRow }

func (f fakeFallbackKeys) FindRecentlyRetired(context.Context, string, string, time.Duration) (*store.KeyRow, error) {
	if len(f.rows) == 0 {
		return nil, nil
	}
	return &f.rows[0], nil
}

func (f fakeFallbackKeys) FallbackKeys(context.Context, string, string, time.Duration) ([]store.KeyRow, error) {
	return f.rows, nil
}

func TestMACVerifier_triesEveryFallbackKeyInOrder__SEC_R2_S2(t *testing.T) {
	pendingZAK, retiredZAK := []byte("pending-zak-key!"), []byte("retired-zak-key!")
	hsmFake := &fakeHSM{
		macToReturn: []byte{1, 2, 3, 4, 5, 6, 7, 8}, // the ACTIVE key does not match
		macForKey: map[string][]byte{
			hex.EncodeToString(pendingZAK): {9, 9, 9, 9, 9, 9, 9, 9},
			hex.EncodeToString(retiredZAK): {0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11},
		},
	}
	keys := fakeFallbackKeys{rows: []store.KeyRow{
		{KeyUnderLMKHex: hex.EncodeToString(pendingZAK)},
		{KeyUnderLMKHex: hex.EncodeToString(retiredZAK)},
	}}
	resp := map[int]string{39: "00", 11: "000123", 64: "aabbccddeeff0011"} // MAC'd under the retired key

	require.True(t, NewMACVerifier(hsmFake, testZAK, keys).Verify(context.Background(), responseMTI, resp),
		"the retired key is tried after the pending one fails")
	require.False(t, NewMACVerifier(hsmFake, testZAK, fakeFallbackKeys{rows: keys.rows[:1]}).Verify(context.Background(), responseMTI, resp))
}
