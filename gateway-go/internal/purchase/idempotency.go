package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/store"
)

// IdempotencyPort persists idempotency_record (docs/04 §2). *store.IdempotencyRepository
// satisfies it.
type IdempotencyPort interface {
	Reserve(ctx context.Context, key, route, requestHash string) (*store.StoredResponse, error)
	AttachRRN(ctx context.Context, key, route, rrn string) error
	Reclaim(ctx context.Context, key, route, requestHash string, staleAfter time.Duration) (bool, error)
	Store(ctx context.Context, key, route, requestHash string, status int, body []byte) error
	Release(ctx context.Context, key, route string) error
}

// staleAfter is how long a reserved key may stay in flight before a retry answers from what its
// request left in tran_log: the send timeout, the detached recording after it, and a margin.
const staleAfter = requestTimeout + persistTimeout + 20*time.Second

// Recover answers a request from the tran_log row of the RRN it sent: the transaction, and whether
// its status is final (worth storing as the key's response).
type Recover[T any] func(ctx context.Context, rrn string) (txn T, final bool, err error)

type reservationKey struct{}

// reservation is the idempotency key a create call holds, carried in its context.
type reservation struct {
	port       IdempotencyPort
	key, route string
	rrn        string
}

// AttachRRN records, on the idempotency key ctx's request holds, the RRN it is about to send, so
// a retry can learn the outcome from tran_log if this request never finishes (#117 review S1).
// Call it before the send; outside Idempotent it does nothing.
func AttachRRN(ctx context.Context, rrn string) error {
	r, ok := ctx.Value(reservationKey{}).(*reservation)
	if !ok {
		return nil
	}
	r.rrn = rrn
	if err := r.port.AttachRRN(ctx, r.key, r.route, rrn); err != nil {
		return fmt.Errorf("attach RRN %s to idempotency key: %w", rrn, err)
	}
	return nil
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

// idemKey is one (key, route) and the request reserved under it.
type idemKey struct {
	port                    IdempotencyPort
	key, route, requestHash string
	status                  int
}

// Idempotent runs create at most once per (key, route). A concurrent or later call with the same
// request replays the stored response; a different request under the same key is
// store.ErrIdempotencyKeyMismatch. When create fails before anything was sent, the key is
// released so the caller may retry it. A request that may have been sent but never recorded its
// response - it failed after the send, or a retry finds its key in flight past staleAfter - is
// answered by fromLog from the RRN it sent; fromLog may be nil for a route that sends nothing.
func Idempotent[T any](ctx context.Context, port IdempotencyPort, key, route, requestHash string, status int, create func(ctx context.Context) (T, error), fromLog Recover[T]) (T, error) {
	k := idemKey{port: port, key: key, route: route, requestHash: requestHash, status: status}
	if answer, done, err := begin(ctx, k, fromLog); done || err != nil {
		return answer, err
	}
	held := &reservation{port: port, key: key, route: route}
	result, err := create(context.WithValue(ctx, reservationKey{}, held))
	// Whatever happened to the caller, the key's fate must be recorded.
	ctx, cancel := Detach(ctx)
	defer cancel()
	if err != nil {
		return failed(ctx, k, held.rrn, fromLog, err)
	}
	if err := storeResponse(ctx, k, result); err != nil {
		return result, afterSend(err)
	}
	return result, nil
}

// begin reserves the key; done means the answer is a replay (or, for a stale key, the recovered
// outcome) and create must not run.
func begin[T any](ctx context.Context, k idemKey, fromLog Recover[T]) (answer T, done bool, err error) {
	stored, err := k.port.Reserve(ctx, k.key, k.route, k.requestHash)
	var pending *store.InProgressError
	if errors.As(err, &pending) && time.Since(pending.ReservedAt) > staleAfter {
		return takeOverStale(ctx, k, pending.RRN, fromLog)
	}
	if err != nil {
		return answer, true, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if stored == nil {
		return answer, false, nil
	}
	if err := json.Unmarshal(stored.Body, &answer); err != nil {
		return answer, true, fmt.Errorf("decode stored response: %w", err)
	}
	return answer, true, nil
}

// takeOverStale settles a key still in flight past staleAfter. With an RRN it may have been sent,
// so it is answered from tran_log; without one nothing was sent (AttachRRN runs before every send),
// so the caller takes the key over and sends. Either way a retry never waits out the 24 h TTL.
func takeOverStale[T any](ctx context.Context, k idemKey, rrn string, fromLog Recover[T]) (answer T, done bool, err error) {
	if rrn != "" {
		if fromLog == nil {
			return answer, true, fmt.Errorf("reserve idempotency key: %w", store.ErrIdempotencyInProgress)
		}
		answer, err = answerFromLog(ctx, k, rrn, fromLog)
		return answer, true, err
	}
	reclaimed, err := k.port.Reclaim(ctx, k.key, k.route, k.requestHash, staleAfter)
	switch {
	case err != nil:
		return answer, true, fmt.Errorf("reclaim idempotency key: %w", err)
	case !reclaimed: // another retry took it over first
		return answer, true, fmt.Errorf("reserve idempotency key: %w", store.ErrIdempotencyInProgress)
	}
	return answer, false, nil
}

// failed releases the key when nothing was sent; otherwise it answers from the RRN sent, if any.
func failed[T any](ctx context.Context, k idemKey, sentRRN string, fromLog Recover[T], err error) (T, error) {
	var zero T
	if !errors.Is(err, ErrAfterSend) {
		if releaseErr := k.port.Release(ctx, k.key, k.route); releaseErr != nil {
			return zero, errors.Join(err, fmt.Errorf("release idempotency key: %w", releaseErr))
		}
		return zero, err
	}
	if fromLog == nil || sentRRN == "" {
		return zero, err
	}
	answer, answerErr := answerFromLog(ctx, k, sentRRN, fromLog)
	if answerErr != nil {
		return zero, errors.Join(err, answerErr)
	}
	return answer, nil
}

// answerFromLog answers from rrn's tran_log row and, once its status is final, stores that as the
// key's response so later replays don't read tran_log again.
func answerFromLog[T any](ctx context.Context, k idemKey, rrn string, fromLog Recover[T]) (T, error) {
	var zero T
	txn, final, err := fromLog(ctx, rrn)
	if err != nil {
		return zero, fmt.Errorf("answer from transaction %s: %w", rrn, err)
	}
	if !final {
		return txn, nil
	}
	if err := storeResponse(ctx, k, txn); err != nil {
		return zero, err
	}
	return txn, nil
}

func storeResponse[T any](ctx context.Context, k idemKey, response T) error {
	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode response for idempotency store: %w", err)
	}
	if err := k.port.Store(ctx, k.key, k.route, k.requestHash, k.status, body); err != nil {
		return fmt.Errorf("store idempotent response: %w", err)
	}
	return nil
}
