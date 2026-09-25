package saf

import (
	"encoding/json"
	"fmt"

	"github.com/mcn/gateway-go/internal/store"
)

// encodePayload JSON-encodes adv and, when key is non-empty, seals it with store.EncryptBytes for
// saf_queue.payload_enc. adv never holds a PAN: DE 2 is added only in the frame being sent.
func encodePayload(key []byte, adv advice) ([]byte, error) {
	plain, err := json.Marshal(adv)
	if err != nil {
		return nil, fmt.Errorf("encode saf fields: %w", err)
	}
	if len(key) == 0 {
		return plain, nil
	}
	return store.EncryptBytes(key, plain)
}

// decodePayload reverses encodePayload. An empty payload decodes to an advice with no fields.
func decodePayload(key, payload []byte) (advice, error) {
	if len(payload) == 0 {
		return advice{Fields: map[int]string{}}, nil
	}
	plain := payload
	if len(key) > 0 {
		decrypted, err := store.DecryptBytes(key, payload)
		if err != nil {
			return advice{}, fmt.Errorf("decrypt saf payload: %w", err)
		}
		plain = decrypted
	}
	var adv advice
	if err := json.Unmarshal(plain, &adv); err != nil {
		return advice{}, fmt.Errorf("decode saf payload: %w", err)
	}
	if adv.Fields == nil {
		adv.Fields = map[int]string{}
	}
	return adv, nil
}
