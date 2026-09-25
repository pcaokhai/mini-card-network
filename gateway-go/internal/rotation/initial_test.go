package rotation

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingKeyStore struct {
	hasActive  bool
	keyType    string
	cryptogram string
	kcv        string
}

func (r *recordingKeyStore) EnsureActive(_ context.Context, keyType, keyUnderLMKHex, kcv string) (bool, error) {
	if r.hasActive {
		return false, nil
	}
	r.keyType, r.cryptogram, r.kcv = keyType, keyUnderLMKHex, kcv
	return true, nil
}

func TestProvisionInitialKey_wrapsUnderLMKAndRecordsKCV__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{}
	clearKey := []byte("0123456789abcdef")

	inserted, kcv, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZAK", clearKey)

	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, "ZAK", ks.keyType)
	require.Equal(t, kcv, ks.kcv)
	require.NotEqual(t, hex.EncodeToString(clearKey), ks.cryptogram, "the clear key is never stored")
}

func TestProvisionInitialKey_noClearKeyIsANoOp__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{}

	inserted, _, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZPK", nil)

	require.NoError(t, err)
	require.False(t, inserted)
	require.Empty(t, ks.keyType)
}

func TestProvisionInitialKey_keepsAnExistingActiveKey__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{hasActive: true}

	inserted, _, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZAK", []byte("0123456789abcdef"))

	require.NoError(t, err)
	require.False(t, inserted)
}
