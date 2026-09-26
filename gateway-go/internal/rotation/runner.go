package rotation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/store"
)

// clearKeyLenBytes is the generated key length (a fixed simulator choice, matching MCN-501's
// existing GENERATE precedent, not a real HSM key-size review).
const clearKeyLenBytes = 16

// keyChangeDE70 is DE 70's value for a key-change network management message
// (docs/03 §7.3: "161 | Key change (new ZPK/ZAK in DE 48 under ZMK, key index in DE 53)").
const keyChangeDE70 = "161"

// Mux sends a request on the acquirer's live issuer connection and awaits its response.
// *isonet.Supervisor satisfies it (the same port purchase.Service already sends through).
type Mux interface {
	NextSTAN() (stan string, ok bool)
	Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error)
}

var (
	// ErrNotServing means Start was called before Serve or after shutdown began.
	ErrNotServing = errors.New("rotation runner is not serving")
	// ErrRotationInProgress means another rotation is still RUNNING: two at once would race to
	// activate their keys.
	ErrRotationInProgress = errors.New("a key rotation is already running")
)

// Runner drives a rotation's GENERATE -> SEND_0800_161 -> PARTNER_CONFIRM -> ACTIVATE sequence.
// Start runs it on a goroutine Serve owns, so POST returns RUNNING at once and the UI follows the
// steps by polling (SEC-G2); Run is the same sequence inline.
type Runner struct {
	repo       *Repository
	keyStore   *store.KeyStoreRepository
	hsm        hsm.Module
	mux        Mux
	zmk        []byte
	onActivate func(ctx context.Context, keyType string)

	sendAttempts int
	sendTimeout  time.Duration

	mu      sync.Mutex
	base    context.Context // Serve's context; nil when not serving
	running bool
	wg      sync.WaitGroup
}

// Option configures optional Runner behaviour.
type Option func(*Runner)

// rcOutcomeUnknown is the 0810 RC ("system malfunction") the issuer answers for any exception,
// even one after the key's activation committed; only another non-"00" RC is a decline.
const rcOutcomeUnknown = "96"

// defaultSendAttempts is how often an unanswered 0800/161 is sent before the outcome is unknown.
const defaultSendAttempts = 3

// defaultSendTimeout bounds one 0800/161 round trip, like the supervisor's echo timeout.
const defaultSendTimeout = 10 * time.Second

// WithSendTimeout bounds each 0800/161 attempt; without it a lost 0810 would wait on Serve's
// context forever (review B1).
func WithSendTimeout(d time.Duration) Option { return func(r *Runner) { r.sendTimeout = d } }

// WithSendAttempts sets how many times an unanswered 0800/161 is sent, with the same key (min 1).
func WithSendAttempts(n int) Option { return func(r *Runner) { r.sendAttempts = max(n, 1) } }

// WithActivationHook calls fn after a new key is activated, so holders of the clear key reload it
// without a restart (SEC-G10).
func WithActivationHook(fn func(ctx context.Context, keyType string)) Option {
	return func(r *Runner) { r.onActivate = fn }
}

// NewRunner builds a Runner. zmk is the clear Zone Master Key used to wrap the new key for
// transport in DE 48 of the 0800 (docs/03 §7.3's key-change convention).
func NewRunner(repo *Repository, keyStore *store.KeyStoreRepository, hsmModule hsm.Module, mux Mux, zmk []byte, opts ...Option) *Runner {
	r := &Runner{repo: repo, keyStore: keyStore, hsm: hsmModule, mux: mux, zmk: zmk, sendAttempts: defaultSendAttempts, sendTimeout: defaultSendTimeout}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Serve owns the background rotations. It first resolves any rotation a crash left RUNNING
// (recoverInterrupted), then accepts Start calls until ctx is cancelled, and returns once
// every running rotation has stopped (root CLAUDE.md §6 rule 9).
func (r *Runner) Serve(ctx context.Context) error {
	if err := r.recoverInterrupted(ctx); err != nil {
		if ctx.Err() != nil {
			return nil // shut down before it started
		}
		return fmt.Errorf("recover interrupted rotations: %w", err)
	}
	r.mu.Lock()
	r.base = ctx
	r.mu.Unlock()
	<-ctx.Done()
	r.mu.Lock()
	r.base = nil // no Start after this point, so wg.Add never races wg.Wait
	r.mu.Unlock()
	r.wg.Wait()
	return nil
}

func (r *Runner) serving() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.base != nil
}

// Start records a RUNNING rotation and returns it at once; the steps run in the background under
// Serve's context, so a shutdown cancels them and the rotation ends FAILED.
func (r *Runner) Start(ctx context.Context, keyType, ownerRef string) (Row, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.base == nil {
		return Row{}, ErrNotServing
	}
	if r.running {
		return Row{}, ErrRotationInProgress
	}
	id, err := r.repo.Create(ctx, keyType)
	if err != nil {
		return Row{}, fmt.Errorf("create rotation: %w", err)
	}
	row, err := r.repo.Get(ctx, id)
	if err != nil {
		return Row{}, err
	}
	r.running = true
	r.wg.Add(1)
	// The steps run under Serve's context, not the request's: the request ends with the 202,
	// while a shutdown must still cancel the rotation.
	//nolint:contextcheck // deliberately Serve's context - see comment above.
	go func(base context.Context) {
		defer r.wg.Done()
		_, _ = r.execute(base, id, keyType, ownerRef) // the outcome is on the row; GET reports it
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}(r.base)
	return row, nil
}

// Run executes all four rotation steps for (keyType, ownerRef) inline and returns the final row.
// On a step failure it marks that step FAILED, the rotation FAILED, and returns the row and error.
func (r *Runner) Run(ctx context.Context, keyType, ownerRef string) (Row, error) {
	id, err := r.repo.Create(ctx, keyType)
	if err != nil {
		return Row{}, fmt.Errorf("create rotation: %w", err)
	}
	return r.execute(ctx, id, keyType, ownerRef)
}

func (r *Runner) execute(ctx context.Context, id int64, keyType, ownerRef string) (Row, error) {
	cryptogram, newRowID, kcv, err := r.runGenerate(ctx, id, keyType, ownerRef)
	if err != nil {
		return r.failStep(ctx, id, StepGenerate, newRowID, err)
	}
	resp, err := r.runSend0800161(ctx, id, keyType, cryptogram)
	if err != nil {
		// No 0810 after every attempt: the issuer may have activated the key, so it stays PENDING
		// (the MAC fallback tries it) until a key-status query can settle it (SEC-G16).
		return r.failStep(ctx, id, StepSend0800161, 0, fmt.Errorf("outcome unknown: %w", err))
	}
	if err := r.runPartnerConfirm(ctx, id, resp); err != nil {
		if resp[39] != "00" {
			return r.failStep(ctx, id, StepPartnerConfirm, newRowID, err) // declined: the key is dead
		}
		return r.failStep(ctx, id, StepPartnerConfirm, 0, err)
	}
	if err := r.runActivate(ctx, id, newRowID); err != nil {
		// The issuer confirmed the key; keep it PENDING rather than retire a key in use (S2).
		return r.failStep(ctx, id, StepActivate, 0, err)
	}
	if r.onActivate != nil {
		r.onActivate(ctx, keyType)
	}

	if err := r.repo.Complete(ctx, id, kcv); err != nil {
		return Row{}, fmt.Errorf("complete rotation: %w", err)
	}
	return r.repo.Get(ctx, id)
}

// runGenerate creates a fresh clear key and returns only its cryptogram under the ZMK for the
// 0800: the clear key is wrapped under the LMK for key_store, KCV'd, wrapped under the ZMK and
// zeroed before this returns, never persisted or logged in the clear (root CLAUDE.md §6 rule 2).
// newRowID is non-zero once the PENDING key_store row exists, even on a later error.
//
// store.EncryptBytes (not hsm.WrapUnderLMK, which is bound to the module's own LMK) wraps the
// clear key under the ZMK directly - the same AES-256-GCM primitive Unwrap/WrapUnderLMK already
// use internally, just keyed by r.zmk instead of the module's LMK.
func (r *Runner) runGenerate(ctx context.Context, id int64, keyType, ownerRef string) (underZMK []byte, newRowID int64, kcv string, err error) {
	clearKey := make([]byte, clearKeyLenBytes)
	defer clear(clearKey)
	if _, genErr := rand.Read(clearKey); genErr != nil {
		return nil, 0, "", fmt.Errorf("generate key: %w", genErr)
	}
	wrappedUnderLMK, err := r.hsm.WrapUnderLMK(clearKey)
	if err != nil {
		return nil, 0, "", fmt.Errorf("wrap under LMK: %w", err)
	}
	kcv, err = r.hsm.ComputeKCV(clearKey)
	if err != nil {
		return nil, 0, "", fmt.Errorf("compute KCV: %w", err)
	}
	underZMK, err = store.EncryptBytes(r.zmk, clearKey)
	if err != nil {
		return nil, 0, "", fmt.Errorf("wrap under ZMK: %w", err)
	}
	newRowID, err = r.keyStore.Insert(ctx, store.KeyRow{
		KeyType: keyType, OwnerRef: ownerRef,
		KeyUnderLMKHex: strings.ToUpper(hex.EncodeToString(wrappedUnderLMK)), KCV: kcv,
	})
	if err != nil {
		return nil, 0, "", fmt.Errorf("insert key_store row: %w", err)
	}
	if err := r.repo.SetNewKey(ctx, id, newRowID); err != nil {
		return nil, newRowID, "", fmt.Errorf("link key to rotation: %w", err)
	}
	if err := r.completeStep(ctx, id, StepGenerate); err != nil {
		return nil, newRowID, "", err
	}
	return underZMK, newRowID, kcv, nil
}

// runSend0800161 sends the new key as a cryptogram under the ZMK in DE 48. Key type travels as a
// literal "ZPK:"/"ZAK:" prefix inside DE 48's own value, not a separate field: MCN-504-ISS's
// Ruling correction (PR #60) found the plan's assumed DE 123 key-type carrier does not exist in
// this project's packager (cfg/iso87ascii.xml has no field 123), and adding one is a contracts/
// change out of scope for a feature branch. This must match issuer-jpos's ReceiveKeyChange
// parsing byte-for-byte: prefix + ":" + lowercase hex of the cryptogram, no DE 53.
//
// A send with no 0810 is repeated up to sendAttempts times with the same cryptogram, so the
// issuer never sees two different keys for one rotation (review S2). An 0810 with RC 96 is
// treated the same way: the issuer answers 96 for any exception, including one after it committed
// the key's activation (SEC-G23), so 96 proves nothing and must never retire the key.
func (r *Runner) runSend0800161(ctx context.Context, id int64, keyType string, underZMK []byte) (map[int]string, error) {
	de48 := keyType + ":" + hex.EncodeToString(underZMK)
	var lastErr error
	for attempt := 0; attempt < r.sendAttempts && ctx.Err() == nil; attempt++ {
		stan, ok := r.mux.NextSTAN()
		if !ok {
			lastErr = errors.New("issuer link not signed on")
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, r.sendTimeout)
		resp, err := r.mux.Send(attemptCtx, "0800", map[int]string{7: nowDE7(), 11: stan, 70: keyChangeDE70, 48: de48})
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("send 0800 (attempt %d): %w", attempt+1, err)
			continue
		}
		if resp[39] == rcOutcomeUnknown {
			lastErr = fmt.Errorf("0810 RC %s (attempt %d): the issuer may have activated the key", rcOutcomeUnknown, attempt+1)
			continue
		}
		if err := r.completeStep(ctx, id, StepSend0800161); err != nil {
			return nil, err
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = ctx.Err()
	}
	return nil, lastErr
}

// runPartnerConfirm checks the 0810's RC (DE 39) - "00" confirms.
func (r *Runner) runPartnerConfirm(ctx context.Context, id int64, resp map[int]string) error {
	if rc := resp[39]; rc != "00" {
		return fmt.Errorf("partner did not confirm: RC %s", rc)
	}
	return r.completeStep(ctx, id, StepPartnerConfirm)
}

// runActivate retires the prior ACTIVE row (store.KeyStoreRepository.Activate, MCN-501), leaving
// it available to FindRecentlyRetired for dualKeyWindow (Ruling 2).
func (r *Runner) runActivate(ctx context.Context, id, newRowID int64) error {
	if err := r.keyStore.Activate(ctx, newRowID); err != nil {
		return fmt.Errorf("activate key: %w", err)
	}
	return r.completeStep(ctx, id, StepActivate)
}

// completeStep marks stepName DONE and writes its audit record.
func (r *Runner) completeStep(ctx context.Context, id int64, stepName string) error {
	if err := r.repo.UpdateStep(ctx, id, stepName, StatusDone); err != nil {
		return fmt.Errorf("mark %s done: %w", stepName, err)
	}
	return r.repo.WriteAudit(ctx, "key_rotation.step", map[string]any{"rotationId": id, "step": stepName, "status": StatusDone})
}

// failStep marks stepName FAILED, the rotation FAILED, retires the never-activated key
// (pendingKeyID, 0 when GENERATE failed), writes an audit record, and returns the rotation's
// current row alongside the original error. It writes with ctx's cancellation stripped: a
// shutdown is exactly when the FAILED state must still land.
func (r *Runner) failStep(ctx context.Context, id int64, stepName string, pendingKeyID int64, cause error) (Row, error) {
	ctx = context.WithoutCancel(ctx)
	if pendingKeyID != 0 {
		_ = r.keyStore.RetirePending(ctx, pendingKeyID)
	}
	_ = r.repo.UpdateStep(ctx, id, stepName, StatusFailed)
	_ = r.repo.WriteAudit(ctx, "key_rotation.step", map[string]any{"rotationId": id, "step": stepName, "status": StatusFailed, "error": cause.Error()})
	_ = r.repo.Fail(ctx, id)
	row, getErr := r.repo.Get(ctx, id)
	if getErr != nil {
		return Row{}, cause
	}
	return row, cause
}

// nowDE7 formats the current time as DE 7 (MMDDhhmmss), matching internal/isonet's convention.
func nowDE7() string { return time.Now().UTC().Format("0102150405") }
