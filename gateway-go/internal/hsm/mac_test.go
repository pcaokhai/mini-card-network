package hsm

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJCEModule_computeMAC_returnsEightBytes__MCN_502_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	zak, _ := hex.DecodeString("3132333435363738393a3b3c3d3e3f40")

	mac, err := m.ComputeMAC([]byte("packed-message-excluding-mac-field"), zak)

	require.NoError(t, err)
	require.Len(t, mac, 8)
}

type macVector struct {
	PackedHexExcludingMAC string `json:"packedHexExcludingMac"`
	ZAKHex                string `json:"zakHex"`
	ExpectedMACHex        string `json:"expectedMacHex"`
}

func TestJCEModule_computeMAC_matchesGoldenVector__MCN_502_AC1(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "iso8583", "vectors", "mac", "0200-with-mac.json"))
	require.NoError(t, err, "requires contracts/ PR from Task 1 merged")
	var v macVector
	require.NoError(t, json.Unmarshal(data, &v))

	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	packed, err := hex.DecodeString(v.PackedHexExcludingMAC)
	require.NoError(t, err)
	zak, err := hex.DecodeString(v.ZAKHex)
	require.NoError(t, err)

	mac, err := m.ComputeMAC(packed, zak)
	require.NoError(t, err)
	require.Equal(t, v.ExpectedMACHex, hex.EncodeToString(mac))
}

func TestJCEModule_computeMAC_matchesGoldenVector0210__MCN_502_AC1(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "iso8583", "vectors", "mac", "0210-with-mac.json"))
	require.NoError(t, err, "requires contracts/ PR from Task 1 merged")
	var v macVector
	require.NoError(t, json.Unmarshal(data, &v))

	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	packed, err := hex.DecodeString(v.PackedHexExcludingMAC)
	require.NoError(t, err)
	zak, err := hex.DecodeString(v.ZAKHex)
	require.NoError(t, err)

	mac, err := m.ComputeMAC(packed, zak)
	require.NoError(t, err)
	require.Equal(t, v.ExpectedMACHex, hex.EncodeToString(mac))
}

func TestJCEModule_computeMAC_rejectsShortZAK__MCN_502_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)

	_, err = m.ComputeMAC([]byte("message"), []byte("tooshort"))
	require.Error(t, err)
}
