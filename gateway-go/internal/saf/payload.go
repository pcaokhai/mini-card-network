package saf

import (
	"encoding/json"
	"fmt"

	"github.com/mcn/gateway-go/internal/store"
)

// encodePayload JSON-encodes fields and, when key is non-empty, seals it with
// store.EncryptBytes for saf_queue.payload_enc (may embed the PAN via DE 2).
func encodePayload(key []byte, fields map[int]string) ([]byte, error) {
	plain, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode saf fields: %w", err)
	}
	if len(key) == 0 {
		return plain, nil
	}
	return store.EncryptBytes(key, plain)
}

// decodePayload reverses encodePayload. An empty payload decodes to an empty field map.
func decodePayload(key, payload []byte) (map[int]string, error) {
	if len(payload) == 0 {
		return map[int]string{}, nil
	}
	plain := payload
	if len(key) > 0 {
		decrypted, err := store.DecryptBytes(key, payload)
		if err != nil {
			return nil, fmt.Errorf("decrypt saf payload: %w", err)
		}
		plain = decrypted
	}
	var fields map[int]string
	if err := json.Unmarshal(plain, &fields); err != nil {
		return nil, fmt.Errorf("decode saf fields: %w", err)
	}
	return fields, nil
}
