package rotation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
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

// Runner drives one rotation's GENERATE -> SEND_0800_161 -> PARTNER_CONFIRM -> ACTIVATE
// sequence, one call = one full run (no background worker - a rotation is a bounded four-step
// sequence, matching MCN-502/503's "no background workers introduced this sprint" precedent).
type Runner struct {
	repo     *Repository
	keyStore *store.KeyStoreRepository
	hsm      hsm.Module
	mux      Mux
	zmk      []byte
}

// NewRunner builds a Runner. zmk is the clear Zone Master Key used to wrap the new key for
// transport in DE 48 of the 0800 (docs/03 §7.3's key-change convention).
func NewRunner(repo *Repository, keyStore *store.KeyStoreRepository, hsmModule hsm.Module, mux Mux, zmk []byte) *Runner {
	return &Runner{repo: repo, keyStore: keyStore, hsm: hsmModule, mux: mux, zmk: zmk}
}

// Run executes all four rotation steps for (keyType, ownerRef) and returns the final row. On a
// step failure it marks that step FAILED, the rotation FAILED, and returns the row and error.
func (r *Runner) Run(ctx context.Context, keyType, ownerRef string) (Row, error) {
	id, err := r.repo.Create(ctx, keyType)
	if err != nil {
		return Row{}, fmt.Errorf("create rotation: %w", err)
	}

	clearKey, newRowID, kcv, err := r.runGenerate(ctx, id, keyType, ownerRef)
	if err != nil {
		return r.failStep(ctx, id, StepGenerate, err)
	}
	resp, err := r.runSend0800161(ctx, id, keyType, clearKey)
	if err != nil {
		return r.failStep(ctx, id, StepSend0800161, err)
	}
	if err := r.runPartnerConfirm(ctx, id, resp); err != nil {
		return r.failStep(ctx, id, StepPartnerConfirm, err)
	}
	if err := r.runActivate(ctx, id, newRowID); err != nil {
		return r.failStep(ctx, id, StepActivate, err)
	}

	if err := r.repo.Complete(ctx, id, kcv); err != nil {
		return Row{}, fmt.Errorf("complete rotation: %w", err)
	}
	return r.repo.Get(ctx, id)
}

// runGenerate creates a fresh clear key, existing only in this call's return value for the rest
// of Run - wrapped under the LMK for key_store, KCV'd, never persisted or logged in the clear
// (root CLAUDE.md §6 rule 2).
func (r *Runner) runGenerate(ctx context.Context, id int64, keyType, ownerRef string) (clearKey []byte, newRowID int64, kcv string, err error) {
	clearKey = make([]byte, clearKeyLenBytes)
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
	newRowID, err = r.keyStore.Insert(ctx, store.KeyRow{
		KeyType: keyType, OwnerRef: ownerRef,
		KeyUnderLMKHex: strings.ToUpper(hex.EncodeToString(wrappedUnderLMK)), KCV: kcv,
	})
	if err != nil {
		return nil, 0, "", fmt.Errorf("insert key_store row: %w", err)
	}
	if err := r.completeStep(ctx, id, StepGenerate); err != nil {
		return nil, 0, "", err
	}
	return clearKey, newRowID, kcv, nil
}

// runSend0800161 sends the new key as a cryptogram under the ZMK in DE 48. Key type travels as a
// literal "ZPK:"/"ZAK:" prefix inside DE 48's own value, not a separate field: MCN-504-ISS's
// Ruling correction (PR #60) found the plan's assumed DE 123 key-type carrier does not exist in
// this project's packager (cfg/iso87ascii.xml has no field 123), and adding one is a contracts/
// change out of scope for a feature branch. This must match issuer-jpos's ReceiveKeyChange
// parsing byte-for-byte: prefix + ":" + lowercase hex of the cryptogram, no DE 53.
//
// store.EncryptBytes (not hsm.WrapUnderLMK, which is bound to the module's own LMK) wraps the
// clear key under the ZMK directly - the same AES-256-GCM primitive Unwrap/WrapUnderLMK already
// use internally, just keyed by r.zmk instead of the module's LMK.
func (r *Runner) runSend0800161(ctx context.Context, id int64, keyType string, clearKey []byte) (map[int]string, error) {
	wrappedUnderZMK, err := store.EncryptBytes(r.zmk, clearKey)
	if err != nil {
		return nil, fmt.Errorf("wrap under ZMK: %w", err)
	}
	stan, ok := r.mux.NextSTAN()
	if !ok {
		return nil, fmt.Errorf("issuer link not signed on")
	}
	resp, err := r.mux.Send(ctx, "0800", map[int]string{
		7: nowDE7(), 11: stan, 70: keyChangeDE70,
		48: keyType + ":" + hex.EncodeToString(wrappedUnderZMK),
	})
	if err != nil {
		return nil, fmt.Errorf("send 0800: %w", err)
	}
	if err := r.completeStep(ctx, id, StepSend0800161); err != nil {
		return nil, err
	}
	return resp, nil
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

// failStep marks stepName FAILED, the rotation FAILED, writes an audit record, and returns the
// rotation's current row alongside the original error.
func (r *Runner) failStep(ctx context.Context, id int64, stepName string, cause error) (Row, error) {
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
