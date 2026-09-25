package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mcn/gateway-go/internal/store"
)

// IdempotencyPort persists idempotency_record (docs/04 §2). *store.IdempotencyRepository
// satisfies it.
type IdempotencyPort interface {
	Reserve(ctx context.Context, key, route, requestHash string) (*store.StoredResponse, error)
	Store(ctx context.Context, key, route, requestHash string, status int, body []byte) error
	Release(ctx context.Context, key, route string) error
}

// ErrAfterSend marks an error raised once the request may already be at the issuer. Its
// idempotency key stays reserved, so a retry with that key can never send the request twice.
var ErrAfterSend = errors.New("the request may have reached the issuer")

// afterSend wraps err with ErrAfterSend (nil stays nil).
func afterSend(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrAfterSend, err)
}

// Idempotent runs create at most once per (key, route). A concurrent or later call with the same
// request replays the stored response; a different request under the same key is
// store.ErrIdempotencyKeyMismatch. When create fails before anything was sent, the key is
// released so the caller may retry it.
func Idempotent[T any](ctx context.Context, port IdempotencyPort, key, route, requestHash string, status int, create func() (T, error)) (T, error) {
	var zero T
	stored, err := port.Reserve(ctx, key, route, requestHash)
	if err != nil {
		return zero, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if stored != nil {
		var replay T
		if err := json.Unmarshal(stored.Body, &replay); err != nil {
			return zero, fmt.Errorf("decode stored response: %w", err)
		}
		return replay, nil
	}

	result, err := create()
	// Whatever happened to the caller, the key's fate must be recorded.
	ctx, cancel := Detach(ctx)
	defer cancel()
	if err != nil {
		if errors.Is(err, ErrAfterSend) {
			return zero, err
		}
		if releaseErr := port.Release(ctx, key, route); releaseErr != nil {
			return zero, errors.Join(err, fmt.Errorf("release idempotency key: %w", releaseErr))
		}
		return zero, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return zero, afterSend(fmt.Errorf("encode response for idempotency store: %w", err))
	}
	if err := port.Store(ctx, key, route, requestHash, status, body); err != nil {
		return zero, afterSend(fmt.Errorf("store idempotent response: %w", err))
	}
	return result, nil
}
