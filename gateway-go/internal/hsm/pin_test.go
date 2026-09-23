package hsm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJCEModule_translatePIN_neverExposesClearPINBlock__MCN_502_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	tpk := make([]byte, 16)
	zpk := make([]byte, 16)
	for i := range tpk {
		tpk[i] = byte(i)
		zpk[i] = byte(i + 1)
	}
	clearPinBlock := []byte{0x04, 0x12, 0x34, 0x56, 0xFF, 0xFF, 0xFF, 0xFF}
	underTPK, err := desECBEncryptBlock(tpk, clearPinBlock) // mirrors what a real POS/TPK encrypt would produce
	require.NoError(t, err)

	underZPK, err := m.TranslatePIN(underTPK, tpk, zpk)

	require.NoError(t, err)
	require.NotEqual(t, underTPK, underZPK)
	// Prove it decrypts back to the same clear PIN block under ZPK - the only allowed way to
	// check correctness without a second exposure point.
	decrypted, err := desECBDecryptBlock(zpk, underZPK)
	require.NoError(t, err)
	require.Equal(t, clearPinBlock, decrypted)
}

func TestJCEModule_translatePIN_rejectsWrongLengthBlock__MCN_502_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	tpk := make([]byte, 16)
	zpk := make([]byte, 16)

	_, err = m.TranslatePIN([]byte("short"), tpk, zpk)
	require.Error(t, err)
}
