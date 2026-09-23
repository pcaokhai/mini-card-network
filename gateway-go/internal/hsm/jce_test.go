package hsm

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

const testLMKHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestNewJCEModule_rejectsEmptyOrInvalidLMK__MCN_501_AC3(t *testing.T) {
	_, err := NewJCEModule("")
	require.ErrorContains(t, err, "LMK")

	_, err = NewJCEModule("not-hex")
	require.Error(t, err)
}

func TestJCEModule_wrapUnwrapRoundTrip__MCN_501_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef")

	wrapped, err := m.WrapUnderLMK(clearKey)
	require.NoError(t, err)
	require.NotEqual(t, clearKey, wrapped)

	unwrapped, err := m.Unwrap(wrapped)
	require.NoError(t, err)
	require.Equal(t, clearKey, unwrapped)
}

func TestJCEModule_computeKCV_sixHexChars__MCN_501_AC2(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef")

	kcv, err := m.ComputeKCV(clearKey)
	require.NoError(t, err)
	require.Len(t, kcv, 6)
	require.Regexp(t, "^[0-9A-F]{6}$", kcv)
}

// TestJCEModule_computeKCV_knownVector proves this implementation follows the same
// zero-block-AES-ECB-encrypt convention MCN-501-ISS.md's Ruling 1 documents, using a
// hand-computed expected value so the issuer side (separate process/language) can be checked
// against the same documented algorithm without a cross-process call.
func TestJCEModule_computeKCV_knownVector__MCN_501_AC2(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)

	key, err := hex.DecodeString("404142434445464748494a4b4c4d4e4f")
	require.NoError(t, err)

	kcv, err := m.ComputeKCV(key)
	require.NoError(t, err)
	require.Equal(t, expectedKCVForTestKey, kcv)
}

func TestJCEModule_neverLeaksClearKeyInErrorsOrWrappedOutput__MCN_501_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("deadbeefdeadbeefdeadbeefdeadbeef")
	clearKeyHex := "deadbeefdeadbeefdeadbeefdeadbeef"

	wrapped, err := m.WrapUnderLMK(clearKey)
	require.NoError(t, err)
	require.NotContains(t, hex.EncodeToString(wrapped), clearKeyHex)

	_, err = m.Unwrap([]byte{1, 2, 3})
	require.Error(t, err)
	require.NotContains(t, err.Error(), clearKeyHex)
}

const expectedKCVForTestKey = "189956"
