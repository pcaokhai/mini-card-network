package purchase

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/store"
)

// dualKeyWindow is MCN-504-AC2's grace period: a ZAK retired by a rotation is still accepted for
// this long after retirement (docs/03 §9 "Reversal grace for new key (DE 70 = 161)").
const dualKeyWindow = 5 * time.Minute

// RetiredKeyFinder looks up a recently-retired key, backing MAC dual-key acceptance during a
// rotation's grace window (MCN-504-AC2). *store.KeyStoreRepository satisfies it.
type RetiredKeyFinder interface {
	FindRecentlyRetired(ctx context.Context, keyType, ownerRef string, within time.Duration) (*store.KeyRow, error)
}

// MACVerifier checks the Retail MAC on an issuer response (MCN-502-AC3). Every flow that sends a
// request verifies its response through it, so none can skip the check (POS-G5).
type MACVerifier struct {
	hsm      hsm.Module
	zak      []byte
	keyStore RetiredKeyFinder
}

// NewMACVerifier builds a MACVerifier. A nil keyStore disables the dual-key retry.
func NewMACVerifier(hsmModule hsm.Module, zak []byte, keyStore RetiredKeyFinder) MACVerifier {
	return MACVerifier{hsm: hsmModule, zak: zak, keyStore: keyStore}
}

// Verify recomputes the Retail MAC over resp (excluding its own DE 64/128) packed as mti and
// compares it to the MAC resp carried. On a mismatch against the current ZAK, it retries once
// against the most recently retired ZAK (if any, within dualKeyWindow) - MCN-504-AC2's dual-key
// acceptance so an in-flight transaction MAC'd under the old ZAK still verifies during a
// rotation's grace window. A response with no MAC field at all fails verification.
func (v MACVerifier) Verify(ctx context.Context, mti string, resp map[int]string) bool {
	macHex, macField, ok := macFieldOf(resp)
	if !ok {
		return false
	}
	respWithoutMAC := make(map[int]string, len(resp))
	for n, val := range resp {
		if n != macField {
			respWithoutMAC[n] = val
		}
	}
	packed, err := iso8583.Pack(mti, respWithoutMAC)
	if err != nil {
		return false
	}
	if v.macMatches(packed, macHex, v.zak) {
		return true
	}
	return v.macMatchesRecentlyRetiredZAK(ctx, packed, macHex)
}

// macMatchesRecentlyRetiredZAK retries verification against the most recently retired ZAK, if
// one retired within dualKeyWindow (MCN-504-AC2); a nil keyStore or no such row skips the retry.
func (v MACVerifier) macMatchesRecentlyRetiredZAK(ctx context.Context, packed, macHex string) bool {
	if v.keyStore == nil {
		return false
	}
	// The simulator holds one global ZAK, so its owner reference is empty (rotationAdapter).
	retired, err := v.keyStore.FindRecentlyRetired(ctx, "ZAK", "", dualKeyWindow)
	if err != nil || retired == nil {
		return false
	}
	keyUnderLMK, err := hex.DecodeString(retired.KeyUnderLMKHex)
	if err != nil {
		return false
	}
	oldZAK, err := v.hsm.Unwrap(keyUnderLMK)
	if err != nil {
		return false
	}
	return v.macMatches(packed, macHex, oldZAK)
}

func (v MACVerifier) macMatches(packed, macHex string, zak []byte) bool {
	expected, err := v.hsm.ComputeMAC([]byte(packed), zak)
	if err != nil {
		return false
	}
	return strings.EqualFold(macHex, hex.EncodeToString(expected))
}

// macFieldOf returns whichever of DE 64/DE 128 is present in fields (MCN-502's Ruling 2: DE 128
// only when a secondary bitmap is present).
func macFieldOf(fields map[int]string) (macHex string, fieldNum int, ok bool) {
	if val, present := fields[64]; present {
		return val, 64, true
	}
	if val, present := fields[128]; present {
		return val, 128, true
	}
	return "", 0, false
}
