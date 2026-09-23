package hsm

import (
	"crypto/aes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mcn/gateway-go/internal/store"
)

// JCEModule is the Go-side LMK simulator: one app-held master key plays the role of an HSM's
// LMK (matching the issuer's JCESecurityModule pattern). Wrap/unwrap reuse
// store.EncryptBytes/DecryptBytes (AES-256-GCM) rather than a second hand-rolled cipher.
type JCEModule struct{ lmk []byte }

// NewJCEModule validates lmkHex (32 bytes hex, matching store.EncryptBytes's AES-256 key size)
// and returns a typed error rather than panicking, so config.Load's fail-fast wiring can wrap it.
func NewJCEModule(lmkHex string) (*JCEModule, error) {
	if lmkHex == "" {
		return nil, fmt.Errorf("hsm: LMK is required")
	}
	lmk, err := hex.DecodeString(lmkHex)
	if err != nil {
		return nil, fmt.Errorf("hsm: LMK is not valid hex: %w", err)
	}
	return &JCEModule{lmk: lmk}, nil
}

// WrapUnderLMK seals clearKey under the LMK.
func (m *JCEModule) WrapUnderLMK(clearKey []byte) ([]byte, error) {
	return store.EncryptBytes(m.lmk, clearKey)
}

// Unwrap reverses WrapUnderLMK.
func (m *JCEModule) Unwrap(keyUnderLMK []byte) ([]byte, error) {
	return store.DecryptBytes(m.lmk, keyUnderLMK)
}

// ComputeKCV is the first 3 bytes of AES-ECB(key=clearKey, plaintext=16 zero bytes), uppercase
// hex (MCN-501-ISS.md's Ruling 1 - both sides of the network follow this exact algorithm).
func (m *JCEModule) ComputeKCV(clearKey []byte) (string, error) {
	block, err := aes.NewCipher(clearKey)
	if err != nil {
		return "", fmt.Errorf("hsm: compute KCV: %w", err)
	}
	var zero, out [aes.BlockSize]byte
	block.Encrypt(out[:], zero[:])
	return strings.ToUpper(hex.EncodeToString(out[:3])), nil
}
