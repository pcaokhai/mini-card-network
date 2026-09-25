package rotation

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/mcn/gateway-go/internal/hsm"
)

// InitialKeyStore registers a key as ACTIVE unless one of that type already is;
// *store.KeyStoreRepository implements it.
type InitialKeyStore interface {
	EnsureActive(ctx context.Context, keyType, keyUnderLMKHex, kcv string) (bool, error)
}

// ProvisionInitialKey stores clearKey (the working key both hosts are configured with) under the
// LMK as the ACTIVE keyType, only when key_store has none, so a fresh stack can MAC from the first
// purchase while a key already set by a rotation is never overwritten. A nil clear key is a no-op.
// The clear key never leaves the HSM boundary unwrapped: only its cryptogram and KCV are stored.
func ProvisionInitialKey(ctx context.Context, keys InitialKeyStore, hsmModule hsm.Module, keyType string, clearKey []byte) (inserted bool, kcv string, err error) {
	if clearKey == nil {
		return false, "", nil
	}
	wrapped, err := hsmModule.WrapUnderLMK(clearKey)
	if err != nil {
		return false, "", fmt.Errorf("wrap initial %s under LMK: %w", keyType, err)
	}
	if kcv, err = hsmModule.ComputeKCV(clearKey); err != nil {
		return false, "", fmt.Errorf("compute initial %s KCV: %w", keyType, err)
	}
	inserted, err = keys.EnsureActive(ctx, keyType, hex.EncodeToString(wrapped), kcv)
	if err != nil {
		return false, "", fmt.Errorf("register initial %s: %w", keyType, err)
	}
	return inserted, kcv, nil
}
