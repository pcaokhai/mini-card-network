package rotation

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mcn/gateway-go/internal/hsm"
)

// InitialKeyStore registers a key as ACTIVE unless one of that type already is, and reports the
// ACTIVE one's KCV; *store.KeyStoreRepository implements it.
type InitialKeyStore interface {
	EnsureActive(ctx context.Context, keyType, keyUnderLMKHex, kcv string) (bool, error)
	ActiveKCV(ctx context.Context, keyType string) (string, error)
}

// Provisioned reports what ProvisionInitialKey did. ActiveKCV is set when a key was already
// ACTIVE; it differs from KCV when a rotation ran or the configured key changed.
type Provisioned struct {
	Inserted  bool
	KCV       string
	ActiveKCV string
}

// ProvisionInitialKey stores clearKey (the working key both hosts are configured with) under the
// LMK as the ACTIVE keyType, only when key_store has none, so a fresh stack can MAC from the first
// purchase while a key already set by a rotation is never overwritten. A nil clear key is a no-op.
// The clear key never leaves the HSM boundary unwrapped: only its cryptogram and KCV are stored.
func ProvisionInitialKey(ctx context.Context, keys InitialKeyStore, hsmModule hsm.Module, keyType string, clearKey []byte) (Provisioned, error) {
	if clearKey == nil {
		return Provisioned{}, nil
	}
	wrapped, err := hsmModule.WrapUnderLMK(clearKey)
	if err != nil {
		return Provisioned{}, fmt.Errorf("wrap initial %s under LMK: %w", keyType, err)
	}
	kcv, err := hsmModule.ComputeKCV(clearKey)
	if err != nil {
		return Provisioned{}, fmt.Errorf("compute initial %s KCV: %w", keyType, err)
	}
	// Uppercase like rotation's cryptograms, so the keys API shows one format.
	inserted, err := keys.EnsureActive(ctx, keyType, strings.ToUpper(hex.EncodeToString(wrapped)), kcv)
	if err != nil {
		return Provisioned{}, fmt.Errorf("register initial %s: %w", keyType, err)
	}
	if inserted {
		return Provisioned{Inserted: true, KCV: kcv}, nil
	}
	active, err := keys.ActiveKCV(ctx, keyType)
	if err != nil {
		return Provisioned{}, fmt.Errorf("read active %s KCV: %w", keyType, err)
	}
	return Provisioned{KCV: kcv, ActiveKCV: active}, nil
}
