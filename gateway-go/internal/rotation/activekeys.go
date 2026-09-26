package rotation

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync/atomic"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/store"
)

// KeyRows lists key_store rows; *store.KeyStoreRepository implements it.
type KeyRows interface {
	List(ctx context.Context) ([]store.KeyRow, error)
}

// ActiveKeys holds the clear ACTIVE ZAK for MAC computation and swaps it when a rotation
// activates a new one, so MACs use the new key without a restart (SEC-G10). It implements
// hsm.ZAKSource. The clear key lives only in memory, unwrapped under the LMK on load.
type ActiveKeys struct {
	rows KeyRows
	hsm  hsm.Module
	zak  atomic.Pointer[[]byte]
}

// NewActiveKeys builds an empty ActiveKeys; call Reload(ctx, "ZAK") to load the key.
func NewActiveKeys(rows KeyRows, hsmModule hsm.Module) *ActiveKeys {
	return &ActiveKeys{rows: rows, hsm: hsmModule}
}

// ActiveZAK returns the current clear ZAK, or nil before one was loaded.
func (k *ActiveKeys) ActiveZAK() []byte {
	if p := k.zak.Load(); p != nil {
		return *p
	}
	return nil
}

// Reload re-reads the ACTIVE key of keyType. Only the ZAK is held in the clear; other types are a
// no-op. On error the current key stays in place.
func (k *ActiveKeys) Reload(ctx context.Context, keyType string) error {
	if keyType != "ZAK" {
		return nil
	}
	rows, err := k.rows.List(ctx)
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}
	for _, row := range rows {
		if row.KeyType != keyType || row.Status != "ACTIVE" {
			continue
		}
		keyUnderLMK, err := hex.DecodeString(row.KeyUnderLMKHex)
		if err != nil {
			return fmt.Errorf("decode %s cryptogram: %w", keyType, err)
		}
		clearKey, err := k.hsm.Unwrap(keyUnderLMK)
		if err != nil {
			return fmt.Errorf("unwrap %s: %w", keyType, err)
		}
		k.zak.Store(&clearKey)
		return nil
	}
	return fmt.Errorf("no ACTIVE %s key in key_store", keyType)
}
