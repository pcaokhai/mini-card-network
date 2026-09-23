package hsm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des" //nolint:gosec // Retail MAC / PIN translation are DES/3DES by ISO 8583 security convention (docs/03 §11), not a choice made here.
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

// ComputeMAC computes ISO 9797-1 algorithm 3 (Retail MAC / X9.19) over
// packedMessageExcludingMACField under zak, per docs/03 §11 and MCN-502's Ruling 2: split zak
// into two single-length DES keys k1/k2, zero-pad the message to a multiple of 8 bytes (ISO
// 9797-1 padding method 1), CBC-encrypt under k1 with a zero IV, decrypt the final block under
// k2, then re-encrypt it under k1. The last step's 8 bytes are the MAC.
func (m *JCEModule) ComputeMAC(packedMessageExcludingMACField []byte, zak []byte) ([]byte, error) {
	if len(zak) != 16 {
		return nil, fmt.Errorf("hsm: ZAK must be 16 bytes (double-length DES), got %d", len(zak))
	}
	k1, k2 := zak[:8], zak[8:16]

	block1, err := des.NewCipher(k1)
	if err != nil {
		return nil, fmt.Errorf("hsm: compute MAC: %w", err)
	}
	block2, err := des.NewCipher(k2)
	if err != nil {
		return nil, fmt.Errorf("hsm: compute MAC: %w", err)
	}

	padded := zeroPad(packedMessageExcludingMACField, des.BlockSize)
	var iv [des.BlockSize]byte
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block1, iv[:]).CryptBlocks(encrypted, padded)
	lastBlock := encrypted[len(encrypted)-des.BlockSize:]

	intermediate := make([]byte, des.BlockSize)
	block2.Decrypt(intermediate, lastBlock)
	mac := make([]byte, des.BlockSize)
	block1.Encrypt(mac, intermediate)
	return mac, nil
}

// TranslatePIN decrypts pinBlockUnderTPK under tpk and re-encrypts it under zpk in one function
// body - the clear PIN block never crosses this call's return boundary (MCN-502's Global
// Constraint / root CLAUDE.md §6.2).
func (m *JCEModule) TranslatePIN(pinBlockUnderTPK []byte, tpk, zpk []byte) ([]byte, error) {
	clearPINBlock, err := desECBDecryptBlock(tpk, pinBlockUnderTPK)
	if err != nil {
		return nil, fmt.Errorf("hsm: translate PIN: decrypt under TPK: %w", err)
	}
	underZPK, err := desECBEncryptBlock(zpk, clearPINBlock)
	if err != nil {
		return nil, fmt.Errorf("hsm: translate PIN: encrypt under ZPK: %w", err)
	}
	return underZPK, nil
}

// zeroPad appends zero bytes so len(data) is a multiple of blockSize (ISO 9797-1 padding
// method 1). Already-aligned input is returned unchanged, matching the copy contract of append.
func zeroPad(data []byte, blockSize int) []byte {
	if len(data)%blockSize == 0 {
		return append([]byte{}, data...)
	}
	padded := make([]byte, len(data), len(data)+blockSize-len(data)%blockSize)
	copy(padded, data)
	for len(padded)%blockSize != 0 {
		padded = append(padded, 0)
	}
	return padded
}

// desCipher builds a single/double/triple-length DES cipher from key: 8 bytes is single DES,
// 16 bytes is 2-key triple DES (K1,K2,K1, the standard 2TDEA expansion), 24 bytes is 3-key
// triple DES. PIN blocks and key material in this codebase use 16-byte (double-length) keys.
func desCipher(key []byte) (cipher.Block, error) {
	switch len(key) {
	case 8:
		return des.NewCipher(key)
	case 16:
		return des.NewTripleDESCipher(append(append([]byte{}, key...), key[:8]...))
	case 24:
		return des.NewTripleDESCipher(key)
	default:
		return nil, fmt.Errorf("hsm: key must be 8, 16 or 24 bytes, got %d", len(key))
	}
}

func desECBDecryptBlock(key, block []byte) ([]byte, error) {
	c, err := desCipher(key)
	if err != nil {
		return nil, err
	}
	if len(block) != c.BlockSize() {
		return nil, fmt.Errorf("hsm: block must be %d bytes, got %d", c.BlockSize(), len(block))
	}
	out := make([]byte, len(block))
	c.Decrypt(out, block)
	return out, nil
}

func desECBEncryptBlock(key, block []byte) ([]byte, error) {
	c, err := desCipher(key)
	if err != nil {
		return nil, err
	}
	if len(block) != c.BlockSize() {
		return nil, fmt.Errorf("hsm: block must be %d bytes, got %d", c.BlockSize(), len(block))
	}
	out := make([]byte, len(block))
	c.Encrypt(out, block)
	return out, nil
}
