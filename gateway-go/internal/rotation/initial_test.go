package rotation

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingKeyStore struct {
	hasActive  bool
	activeKCV  string
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

func (r *recordingKeyStore) ActiveKCV(context.Context, string) (string, error) {
	return r.activeKCV, nil
}

func TestProvisionInitialKey_wrapsUnderLMKAndRecordsKCV__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{}
	clearKey := []byte("0123456789abcdef")

	got, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZAK", clearKey)

	require.NoError(t, err)
	require.True(t, got.Inserted)
	require.Equal(t, "ZAK", ks.keyType)
	require.Equal(t, got.KCV, ks.kcv)
	require.NotEqual(t, hex.EncodeToString(clearKey), ks.cryptogram, "the clear key is never stored")
}

func TestProvisionInitialKey_noClearKeyIsANoOp__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{}

	got, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZPK", nil)

	require.NoError(t, err)
	require.False(t, got.Inserted)
	require.Empty(t, ks.keyType)
}

func TestProvisionInitialKey_keepsAnExistingActiveKey__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{hasActive: true}

	got, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZAK", []byte("0123456789abcdef"))

	require.NoError(t, err)
	require.False(t, got.Inserted)
}

// A ZAK_HEX that no longer matches key_store (the issuer was reset, or a rotation ran) makes every
// MAC fail; startup reports both KCVs instead of leaving it to be found per purchase.
func TestProvisionInitialKey_reportsTheActiveKCVWhenItDiffers__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{hasActive: true, activeKCV: "ABCDEF"}

	got, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZAK", []byte("0123456789abcdef"))

	require.NoError(t, err)
	require.False(t, got.Inserted)
	require.Equal(t, "ABCDEF", got.ActiveKCV)
	require.NotEqual(t, got.KCV, got.ActiveKCV)
}

func TestProvisionInitialKey_storesTheCryptogramUppercaseLikeRotation__MCN_002(t *testing.T) {
	ks := &recordingKeyStore{}

	_, err := ProvisionInitialKey(context.Background(), ks, fakeHSM{}, "ZAK", []byte("0123456789abcdef"))

	require.NoError(t, err)
	require.Equal(t, strings.ToUpper(ks.cryptogram), ks.cryptogram)
}
