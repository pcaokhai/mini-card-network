# MCN-504-GW Dynamic key exchange (gateway) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/gw-504`, branch `feat/MCN-504-gw-key-rotation`. Requires MCN-502, MCN-503 merged (both halves of Sprint 6's security lane — this plan extends `internal/hsm.Module`/`JCEModule` and `store.KeyStoreRepository`, both already carrying PIN/MAC operations). Runs in parallel with **MCN-504-ISS** (disjoint directories, disjoint DB schemas — gateway never reads `issuer.key_store`). One story (5 pts) split across two lanes; see `MCN-504-ISS.md` for the issuer half and Ruling 1 below for the shared rotation-state vocabulary both lanes build against.

**Goal:** `POST /v1/keys/acquirer/rotations` runs a persisted GENERATE → SEND_0800_161 → PARTNER_CONFIRM → ACTIVATE workflow that rotates the gateway's ZPK (or ZAK) without downtime: both the old and new key are accepted for 5 minutes after activation, transactions during that window succeed, and every step writes an audit record.

**Architecture:** A new `acquirer.key_rotation(id, key_type, status, steps JSONB)` table (already listed in `docs/05-data-model.md` §4 as owed to MCN-504) backs a new `internal/rotation` package: `rotation.Repository` (persist/load rotation rows, same `pgx` style as `store.KeyStoreRepository`) and `rotation.Runner` (drives the four-step state machine, one step per call so each step is independently retriable and auditable — no single long-running goroutine holding the whole rotation). `internal/hsm.Module` (from MCN-501/502) is reused unchanged for `GENERATE` (a fresh clear key via `crypto/rand`, wrapped with `WrapUnderLMK`, KCV via `ComputeKCV`) — no new crypto primitive is added, only orchestration around the existing ones. `SEND_0800_161` sends a real 0800 network management message with DE 70 = `161` and DE 48 = the new key as a cryptogram under ZMK (per `docs/03` §7.3's key-change row and §11's "Key change" convention) over the existing `internal/mux` link the purchase flow already uses — reuses `internal/iso8583.Pack`/the MUX's existing `SendAndAwait`, not a new transport. `PARTNER_CONFIRM` is the 0810 response to that 0800 (RC `00` = confirmed). `ACTIVATE` calls `store.KeyStoreRepository.Activate` (already retires-previous, from MCN-501) but does **not** immediately drop the old key from acceptance — a new `dualKeyWindow` (5 minutes, per `docs/03` §9's "Reversal grace for new key (DE 70 = 161)" timer) keeps the just-retired row's unwrapped key available to `purchase.Service`'s MAC/PIN-translate paths until it expires, so an in-flight transaction MAC'd under the old ZAK still verifies. `GET /v1/keys/acquirer/rotations/{rotationId}` and `POST /v1/keys/acquirer/rotations` are new chi routes in `internal/api/rotations.go`, same `MountX` pattern as `internal/api/keys.go`.

**Tech Stack:** Go 1.24, `pgx`, `crypto/rand`, existing `internal/hsm`, `internal/store`, `internal/mux`, `internal/iso8583`, chi.

**Spec:** `docs/06-user-stories.md` MCN-504-AC1/AC2/AC3, `contracts/openapi.yaml:407-419` (`POST /v1/keys/acquirer/rotations`, request `{keyType: ZPK|ZAK}`, `202` → `KeyRotation`) and `:420-426` (`GET /v1/keys/acquirer/rotations/{rotationId}`) and `:789-806` (`KeyRotation` schema: `rotationId`, `keyType`, `status: RUNNING|COMPLETED|FAILED`, `newKcv`, `steps[].name: GENERATE|SEND_0800_161|PARTNER_CONFIRM|ACTIVATE`, `steps[].status: PENDING|DONE|FAILED`, `steps[].completedAt`), `docs/03-iso8583-interface-spec.md` §3 (DE 48/DE 53 key-change format), §7.3's "161 | Key change (new ZPK/ZAK in DE 48 under ZMK, key index in DE 53) | Issuer or acquirer" row, §9's "Reversal grace for new key (DE 70 = 161) | 5 min old key accepted | Both" timer, §11's "Key change" bullet, `docs/05-data-model.md` §4's `key_rotation(id, key_type, status, steps JSONB)` row, root `CLAUDE.md` §6 rule 2 (PCI DSS).

## Global Constraints

- The clear new key exists only inside `GENERATE`'s step body (generated, wrapped, KCV'd, discarded) — never persisted or logged in the clear, per `MCN-501-GW.md`'s Global Constraints (unchanged).
- `key_rotation.steps` is `JSONB`, shaped exactly as `KeyRotation.steps[]` in `contracts/openapi.yaml` — the API response is a near-direct projection of the row, not a separately-maintained shape.
- Dual-key acceptance window is exactly 5 minutes (`docs/03` §9), a named constant (`dualKeyWindow = 5 * time.Minute`), not a magic literal inlined at each call site.
- Every step transition (`PENDING → DONE` or `PENDING → FAILED`) writes one `audit_log`-style record — reuse the existing `issuer.audit_log`-equivalent on the acquirer side if one exists (checked in Task 1; if the acquirer schema has no audit table yet, this plan adds `acquirer.audit_log(id, occurred_at, event_type, detail JSONB)`, append-only, matching `docs/05-data-model.md` §3's "Audit: append-only tables are protected by triggers" rule).

## Ruling 1: rotation-state vocabulary — fixed once here, `MCN-504-ISS.md` must match exactly

`contracts/openapi.yaml`'s `KeyRotation.steps[].name` enum (`GENERATE`, `SEND_0800_161`, `PARTNER_CONFIRM`, `ACTIVATE`) is the one and only vocabulary either lane uses for rotation step names — confirmed already fixed in the contract (not invented per-lane, unlike MCN-501's KCV convention which the contract left unspecified). Both `rotation.Runner` (this plan) and the issuer's equivalent participant (`MCN-504-ISS.md`) name their internal state-machine steps identically to this enum, so a rotation's audit trail and the WEB rotation stepper (MCN-505, Sprint 7) read the same four names regardless of which side initiated the rotation. **Who initiates**: the gateway (acquirer) always initiates in v1 — `POST /v1/keys/acquirer/rotations` is a gateway-only endpoint (no equivalent `POST /v1/keys/issuer/rotations` exists in `contracts/openapi.yaml`); the issuer's role is purely to receive `SEND_0800_161`, respond `PARTNER_CONFIRM` (0810 RC 00), and activate its own copy of the new key on its own 5-minute dual-key window — `MCN-504-ISS.md` implements the responder half only, not a second initiator. Cost if this vocabulary drifts: MCN-505's rotation stepper (Sprint 7, same sprint) renders `steps[].name` directly as UI labels — a mismatched name here would either 404 the stepper's icon-per-step mapping or render a raw unmapped string, caught immediately in that story's own integration, not silently.

## Ruling 2: dual-key acceptance is a time-boxed read, not a second ACTIVE row

`uq_active_key`/`uq_acq_active_key` (MCN-501) allow exactly one `ACTIVE` row per `(key_type, owner_ref)` — this plan does not weaken that constraint or add a second concurrently-`ACTIVE` status. Instead, `store.KeyStoreRepository` gains one new read method, `FindRecentlyRetired(ctx, keyType, ownerRef string, within time.Duration) (*KeyRow, error)`, returning the most recently `RETIRED` row if `retired_at` is within `within` of now — `purchase.Service`'s MAC/PIN-translate paths try the `ACTIVE` key first and, only on a verification failure, retry once against a `FindRecentlyRetired` result before giving up with RC 96. This keeps `KeyStoreRepository`'s existing single-`ACTIVE`-row invariant (MCN-501's Ruling, unchanged) while still accepting the old key for the grace window — the "dual acceptance" lives in the read/retry policy, not in the schema.

## Ruling 3: key-type carrier — DE 123 does not exist, key type travels as a prefix in DE 48 itself (correction, from MCN-504-ISS.md PR #60)

MCN-504-ISS's implementation (PR #60, merged) found that this plan's original DE 123 key-type carrier assumption does not exist in this project's packager (`cfg/iso87ascii.xml` defines no field 123), and adding one is a `contracts/` change out of scope for either feature branch (root `CLAUDE.md` §9). The issuer's `ReceiveKeyChange` (`issuer-jpos/src/main/java/io/mcn/issuer/adapter/iso/ReceiveKeyChange.java`) instead parses DE 48's own value as `"ZPK:"`/`"ZAK:"` literal prefix + hex cryptogram (`field48.substring(0, sep)` / `field48.substring(sep + 1)`, split on the first `:`), with DE 53 unused. This gateway lane's `rotation.Runner.runSend0800161` (`internal/rotation/runner.go`) was updated to emit that exact shape — `keyType + ":" + hex.EncodeToString(wrappedUnderZMK)` in DE 48, no DE 53 — confirmed compatible with a decrypt-round-trip test (`TestRunner_run_emitsDE48AsKeyTypePrefixPlusHexCryptogram__MCN_504_AC1`) that asserts the lowercase-hex-no-DE-53 shape byte-for-byte against what `ReceiveKeyChange`'s parser expects. This also corrected a real bug in the original implementation: `runSend0800161` was calling `hsm.WrapUnderLMK` (bound to the module's own LMK) instead of wrapping under the ZMK at all — it now uses `store.EncryptBytes(r.zmk, clearKey)` directly, the same AES-256-GCM primitive `WrapUnderLMK`/`Unwrap` already use internally, just keyed by the ZMK instead of the LMK.

## File map

| Action | Path (under `gateway-go/`) |
| --- | --- |
| Create | `migrations/00005_key_rotation.sql` |
| Create | `internal/rotation/repository.go` (`Repository`), `internal/rotation/runner.go` (`Runner`), tests |
| Modify | `internal/store/keystore.go` (`FindRecentlyRetired`), test |
| Modify | `internal/hsm/module.go`/`jce.go` — no interface change (Task 2 confirms `WrapUnderLMK`/`ComputeKCV` are reused as-is) |
| Modify | `internal/purchase/service.go` (dual-key retry on MAC verify) |
| Create | `internal/api/rotations.go` (`MountRotations`), test |
| Modify | `cmd/gateway/main.go` (wire `rotation.Repository`, `rotation.Runner`, mount rotations route) |

---

### Task 1: `migrations/00005_key_rotation.sql`

**Files:** `migrations/00005_key_rotation.sql`

- [ ] **Step 1:** Confirm the acquirer schema has no audit table yet (`grep -n "audit_log\|CREATE TABLE acquirer" docs/assets/baseline-schema.sql`) — the baseline schema's acquirer section has no `audit_log`, so this migration adds both `key_rotation` and `audit_log`.

```sql
-- +goose Up
CREATE TABLE key_rotation (
    id           BIGSERIAL PRIMARY KEY,
    key_type     TEXT NOT NULL CHECK (key_type IN ('ZPK','ZAK')),
    status       TEXT NOT NULL CHECK (status IN ('RUNNING','COMPLETED','FAILED')),
    steps        JSONB NOT NULL,
    new_kcv      CHAR(6),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    event_type  TEXT NOT NULL,
    detail      JSONB NOT NULL
);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE key_rotation;
```

- [ ] **Step 2: Commit**

```bash
git add gateway-go/migrations/00005_key_rotation.sql
git commit -m "feat(gw): key_rotation, audit_log tables (MCN-504)"
```

---

### Task 2: `rotation.Repository` (AC1, AC3)

**Files:** `internal/rotation/repository.go`, test

**Interfaces:** `Step{Name string; Status string; CompletedAt *time.Time}` (JSON tags `name`/`status`/`completedAt`, matching `KeyRotation.steps[]` exactly — Ruling 1). `Row{ID int64; KeyType string; Status string; Steps []Step; NewKCV *string; CreatedAt, UpdatedAt time.Time}`. `NewRepository(pool *store.Pool) *Repository`; `.Create(ctx, keyType string) (int64, error)` (inserts `status='RUNNING'`, `steps` = all four names `PENDING`); `.UpdateStep(ctx, id int64, stepName, stepStatus string) error` (loads `steps` JSONB, flips the named step's status and `completedAt` when `DONE`, writes it back — one row-level `UPDATE ... WHERE id = $1`, no separate steps table, matching `docs/05-data-model.md`'s `steps JSONB` column choice); `.Complete(ctx, id int64, newKCV string) error` (`status='COMPLETED'`, `new_kcv=$2`); `.Fail(ctx, id int64) error` (`status='FAILED'`); `.Get(ctx, id int64) (Row, error)`; `.WriteAudit(ctx, eventType string, detail any) error` (marshals `detail` to JSONB, inserts into `audit_log`).

- [ ] **Step 1: Write the failing test**

```go
package rotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepository_createThenUpdateStepsThenComplete__MCN_504_AC1_AC3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	id, err := repo.Create(ctx, "ZPK")
	require.NoError(t, err)

	row, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "RUNNING", row.Status)
	require.Len(t, row.Steps, 4)
	for _, s := range row.Steps {
		require.Equal(t, "PENDING", s.Status)
	}

	require.NoError(t, repo.UpdateStep(ctx, id, "GENERATE", "DONE"))
	row, err = repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "DONE", stepStatus(row.Steps, "GENERATE"))
	require.NotNil(t, stepCompletedAt(row.Steps, "GENERATE"))

	require.NoError(t, repo.Complete(ctx, id, "AABBCC"))
	row, err = repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "COMPLETED", row.Status)
	require.Equal(t, "AABBCC", *row.NewKCV)

	require.NoError(t, repo.WriteAudit(ctx, "key_rotation.completed", map[string]any{"rotationId": id}))
}

func stepStatus(steps []Step, name string) string {
	for _, s := range steps {
		if s.Name == name {
			return s.Status
		}
	}
	return ""
}

func stepCompletedAt(steps []Step, name string) *string {
	for _, s := range steps {
		if s.Name == name && s.CompletedAt != nil {
			v := s.CompletedAt.String()
			return &v
		}
	}
	return nil
}
```

Run: `go test ./internal/rotation/... -v` → fails (`rotation` package doesn't exist).

- [ ] **Step 2: Implement** `repository.go`: `Create` marshals a fixed four-`Step` slice (`GENERATE`, `SEND_0800_161`, `PARTNER_CONFIRM`, `ACTIVATE`, each `PENDING`) to JSONB via `json.Marshal`, `INSERT ... RETURNING id`. `UpdateStep` runs `SELECT steps FROM key_rotation WHERE id=$1 FOR UPDATE` inside a transaction, unmarshals, mutates the matching entry, re-marshals, `UPDATE key_rotation SET steps=$1, updated_at=now() WHERE id=$2`. `Get` unmarshals `steps` on read. `WriteAudit` is a one-line `INSERT INTO audit_log (event_type, detail) VALUES ($1, $2)`.

Run: `go test ./internal/rotation/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/rotation/repository.go gateway-go/internal/rotation/repository_test.go
git commit -m "feat(gw): rotation.Repository - key_rotation CRUD, audit_log writes (MCN-504)"
```

---

### Task 3: `store.KeyStoreRepository.FindRecentlyRetired` (AC2)

**Files:** `internal/store/keystore.go` (extend), test

- [ ] **Step 1: Write the failing test**

```go
func TestKeyStoreRepository_findRecentlyRetiredWithinWindow__MCN_504_AC2(t *testing.T) {
	pool := newTestPool(t)
	repo := NewKeyStoreRepository(pool)
	ctx := context.Background()

	firstID, err := repo.Insert(ctx, KeyRow{KeyType: "ZAK", OwnerRef: "gw-link-01", KeyUnderLMKHex: "aa", KCV: "AAAAAA"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, firstID))

	secondID, err := repo.Insert(ctx, KeyRow{KeyType: "ZAK", OwnerRef: "gw-link-01", KeyUnderLMKHex: "bb", KCV: "BBBBBB"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, secondID)) // retires firstID

	recent, err := repo.FindRecentlyRetired(ctx, "ZAK", "gw-link-01", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, recent)
	require.Equal(t, firstID, recent.ID)

	stale, err := repo.FindRecentlyRetired(ctx, "ZAK", "gw-link-01", 0)
	require.NoError(t, err)
	require.Nil(t, stale)
}
```

Run: `go test ./internal/store/... -run FindRecentlyRetired` → fails.

- [ ] **Step 2: Implement**: `FindRecentlyRetired(ctx, keyType, ownerRef string, within time.Duration) (*KeyRow, error)` — `SELECT ... FROM key_store WHERE key_type=$1 AND coalesce(owner_ref,'')=$2 AND status='RETIRED' AND retired_at > now() - $3::interval ORDER BY retired_at DESC LIMIT 1`, returns `nil, nil` on no rows (not an error — "no recently-retired key" is the expected steady state).

Run: `go test ./internal/store/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/store/keystore.go gateway-go/internal/store/keystore_test.go
git commit -m "feat(gw): KeyStoreRepository.FindRecentlyRetired - dual-key acceptance window read (MCN-504)"
```

---

### Task 4: `rotation.Runner` — GENERATE → SEND_0800_161 → PARTNER_CONFIRM → ACTIVATE (AC1, AC2, AC3)

**Files:** `internal/rotation/runner.go`, test

**Interfaces:** `Mux interface{ SendAndAwait(ctx context.Context, msg *iso8583.Message, timeout time.Duration) (*iso8583.Message, error) }` (the existing MUX send port, name confirmed against `internal/mux`'s real interface before use — adjust if the real method differs). `Runner{ repo *Repository; keyStore *store.KeyStoreRepository; hsm hsm.Module; mux Mux; zmk []byte }`. `NewRunner(...) *Runner`; `.Run(ctx context.Context, keyType, ownerRef string) (Row, error)` — runs all four steps sequentially, calling `repo.UpdateStep` after each, `repo.WriteAudit` on every transition, `repo.Complete`/`repo.Fail` at the end; returns the final `Row` (so the HTTP handler can respond `202` with it immediately — this method itself is synchronous for v1, matching MCN-502/503's "no background workers introduced this sprint" precedent, since a rotation is a bounded four-step sequence, not a long poll).

- [ ] **Step 1: Write the failing test**

```go
package rotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeHSM struct{}

func (fakeHSM) WrapUnderLMK(k []byte) ([]byte, error)         { return append([]byte("wrapped:"), k...), nil }
func (fakeHSM) Unwrap(k []byte) ([]byte, error)               { return k, nil }
func (fakeHSM) ComputeKCV(k []byte) (string, error)           { return "AABBCC", nil }
func (fakeHSM) ComputeMAC(m, z []byte) ([]byte, error)        { return make([]byte, 8), nil }
func (fakeHSM) TranslatePIN(p, t, z []byte) ([]byte, error)   { return p, nil }

type fakeMux struct{ confirmed bool }

func (f *fakeMux) SendAndAwait(ctx context.Context, msg *TestMessage, timeout time.Duration) (*TestMessage, error) {
	f.confirmed = true
	return &TestMessage{MTI: "0810", ResponseCode: "00"}, nil
}

func TestRunner_run_completesAllFourStepsAndActivates__MCN_504_AC1_AC3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := &fakeMux{}
	runner := NewRunner(repo, keyStore, fakeHSM{}, mux, []byte("zmk-test-key-32-bytes-----------"))

	row, err := runner.Run(context.Background(), "ZPK", "gw-link-01")

	require.NoError(t, err)
	require.Equal(t, "COMPLETED", row.Status)
	require.Equal(t, "AABBCC", *row.NewKCV)
	require.True(t, mux.confirmed)
	for _, s := range row.Steps {
		require.Equal(t, "DONE", s.Status)
	}

	keys, err := keyStore.List(context.Background())
	require.NoError(t, err)
	require.Contains(t, keyStatusesFor(keys, "ZPK"), "ACTIVE")
}

func TestRunner_run_marksFailedWhenPartnerConfirmErrors__MCN_504_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := &erroringMux{}
	runner := NewRunner(repo, keyStore, fakeHSM{}, mux, []byte("zmk-test-key-32-bytes-----------"))

	row, err := runner.Run(context.Background(), "ZAK", "gw-link-01")

	require.Error(t, err)
	require.Equal(t, "FAILED", row.Status)
	require.Equal(t, "DONE", stepStatus(row.Steps, "GENERATE"))
	require.Equal(t, "FAILED", stepStatus(row.Steps, "SEND_0800_161"))
}
```

Check: `TestMessage`/`iso8583.Message`, `erroringMux`, and `keyStatusesFor` are adjusted to the real `internal/mux`/`internal/iso8583` types before this test is finalized — grep the real MUX send signature (`grep -n "func.*Mux.*Send\|SendAndAwait" gateway-go/internal/mux/*.go`) and align `Runner`'s `Mux` interface and this test's fakes to it exactly, replacing the placeholder `TestMessage` type with the real `iso8583.Message` (or whatever the confirmed real type is named).

Run: `go test ./internal/rotation/... -run TestRunner` → fails.

- [ ] **Step 2: Implement** `runner.go`: `Run` calls `repo.Create(ctx, keyType)` first, then for each step in order: `GENERATE` — `crypto/rand.Read` a 16-byte clear key, `hsm.WrapUnderLMK`, `hsm.ComputeKCV`, `keyStore.Insert` (status `PENDING`), hold the clear key and new row id as locals; `SEND_0800_161` — build a 0800 message with DE 70=`161`, DE 48 = the new clear key wrapped under `zmk` (reusing `hsm`'s wrap primitive with `zmk` in place of the LMK — same function, different key, per the ISO spec's "new key as a cryptogram under ZMK in DE 48" convention), DE 53 = a key index, `mux.SendAndAwait`; `PARTNER_CONFIRM` — inspect the 0810's RC, `00` = confirmed, anything else fails the step; `ACTIVATE` — `keyStore.Activate(ctx, newRowID)` (retires the prior `ACTIVE` row automatically, per MCN-501's existing behavior — this is what makes the retired row available to `FindRecentlyRetired` in Task 3). After each step, `repo.UpdateStep` + `repo.WriteAudit(ctx, "key_rotation.step", map[string]any{"rotationId": id, "step": name, "status": status})`. On any step's error, `repo.UpdateStep(ctx, id, name, "FAILED")`, `repo.Fail(ctx, id)`, return the error and the row (test checks it directly rather than a second `Get`). On full success, `repo.Complete(ctx, id, kcv)`.

Run: `go test ./internal/rotation/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/rotation/runner.go gateway-go/internal/rotation/runner_test.go
git commit -m "feat(gw): rotation.Runner - GENERATE/SEND_0800_161/PARTNER_CONFIRM/ACTIVATE (MCN-504)"
```

---

### Task 5: Dual-key retry in `purchase.Service` MAC verification (AC2)

**Files:** `internal/purchase/service.go` (extend), test

- [ ] **Step 1: Write the failing test**

```go
func TestSendPurchase_incomingMACVerifiesAgainstRecentlyRetiredZAKDuringRotationWindow__MCN_504_AC2(t *testing.T) {
	oldZAK := []byte("old-zak-test-key")
	newZAK := []byte("new-zak-test-key")
	hsmFake := &fakeHSM{macToReturn: computeFakeMAC(oldZAK)} // response MAC'd under the just-retired key
	keyStoreFake := &fakeKeyStoreWithRecentlyRetired{active: newZAK, recentlyRetired: oldZAK}
	svc := newTestService(t, withHSM(hsmFake), withActiveZAK(newZAK), withKeyStore(keyStoreFake))

	txn, err := svc.CreatePurchase(context.Background(), samplePurchaseRequest(), "idem-rotation-1")

	require.NoError(t, err)
	require.NotEqual(t, "96", txn.ResponseCode) // verified on the fallback retired-key retry, not rejected
}
```

Check: `fakeKeyStoreWithRecentlyRetired`, `withActiveZAK`, `withKeyStore`, `computeFakeMAC` are small test doubles added to `service_test.go`, matching MCN-502's `fakeHSM`/`withHSM` test-double conventions — confirm `newTestService`'s current real signature (post-MCN-502) before extending it.

Run: `go test ./internal/purchase/... -run RecentlyRetired` → fails.

- [ ] **Step 2: Implement**: `Service` gains a `keyStore *store.KeyStoreRepository` field (already constructible from existing wiring). In the incoming-0210 MAC-verify path (MCN-502 Task 4), on a mismatch against `s.zak`, before setting RC 96, call `s.keyStore.FindRecentlyRetired(ctx, "ZAK", s.ownerRef, dualKeyWindow)`; if non-nil, unwrap it via `s.hsm.Unwrap` and recompute the MAC once against that key — a match clears the mismatch (no RC 96, no reversal); no match (or no recently-retired row) falls through to the existing RC 96 + reversal path unchanged. Same retry added to `TranslatePIN`'s TPK/ZPK lookups where applicable (a PIN-block translation during an active ZPK rotation must also accept the recently-retired ZPK for the same 5-minute window).

Run: `go test ./internal/purchase/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/purchase/service.go gateway-go/internal/purchase/service_test.go
git commit -m "feat(gw): dual-key acceptance retry for MAC verify and PIN translate during rotation (MCN-504)"
```

---

### Task 6: `POST /v1/keys/acquirer/rotations`, `GET .../{rotationId}`, 50 TPS integration test, main.go wiring (AC1, AC2, AC3)

**Files:** `internal/api/rotations.go`, test; `cmd/gateway/main.go` (extend); `internal/rotation/runner_integration_test.go` (new, build-tagged `integration`)

- [ ] **Step 1: Write the failing tests**

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/rotation"
)

type fakeRotator struct{ result rotation.Row }

func (f *fakeRotator) StartRotation(ctx context.Context, keyType string) (rotation.Row, error) {
	return f.result, nil
}
func (f *fakeRotator) GetRotation(ctx context.Context, id int64) (rotation.Row, error) {
	return f.result, nil
}

func TestPostKeysAcquirerRotations_returns202WithKeyRotation__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 1, KeyType: "ZPK", Status: "COMPLETED"}})

	body, _ := json.Marshal(map[string]string{"keyType": "ZPK"})
	req := httptest.NewRequest(http.MethodPost, "/v1/keys/acquirer/rotations", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "11111111-1111-1111-1111-111111111111")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Contains(t, rec.Body.String(), `"keyType":"ZPK"`)
}

func TestGetKeysAcquirerRotation_returns200__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 7, KeyType: "ZAK", Status: "RUNNING"}})

	req := httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer/rotations/7", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"RUNNING"`)
}
```

Run: `go test ./internal/api/... -run Rotation` → fails.

- [ ] **Step 2: Implement** `rotations.go`: `Rotator interface{ StartRotation(ctx, keyType string) (rotation.Row, error); GetRotation(ctx, id int64) (rotation.Row, error) }`; `MountRotations(r chi.Router, svc Rotator)` registers `POST /v1/keys/acquirer/rotations` (decodes `{keyType}`, requires `Idempotency-Key` header per the existing pattern in `purchases.go`, calls `svc.StartRotation`, responds `202` with the `KeyRotation` JSON shape — `rotationId` as a string per the contract, `steps[]` mapped from `rotation.Step`) and `GET /v1/keys/acquirer/rotations/{rotationId}` (parses the path param to `int64`, calls `svc.GetRotation`, `200`). `rotation.Runner.Run`'s signature is adapted with a thin `StartRotation`/`GetRotation` wrapper in `cmd/gateway/main.go` (or directly on `Runner` if its existing method names already fit — confirm before adding a wrapper).

Run: `go test ./internal/api/... -v` → PASS.

- [ ] **Step 3: Write the 50 TPS integration test** (build-tagged, runs against the real docker-compose stack per `docs/06`'s AC2 "integration test at 50 TPS")

```go
//go:build integration

package rotation_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRotation_transactionsAt50TPSSucceedDuringRotation__MCN_504_AC2(t *testing.T) {
	client := newIntegrationHTTPClient(t) // hits the real running gateway, per docs/06 §"make up"

	rotationDone := make(chan struct{})
	go func() {
		defer close(rotationDone)
		startRotation(t, client, "ZAK")
	}()

	var failures int64
	var wg sync.WaitGroup
	deadline := time.Now().Add(3 * time.Second) // ~50 TPS * 3s = 150 purchases, well inside the 5-minute dual-key window
	for time.Now().Before(deadline) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !sendSamplePurchase(t, client) {
				atomic.AddInt64(&failures, 1)
			}
		}()
		time.Sleep(20 * time.Millisecond) // ~50 TPS
	}
	wg.Wait()
	<-rotationDone

	require.Zero(t, atomic.LoadInt64(&failures), "no purchase should fail RC 96 during a rotation with a valid dual-key window")
}
```

Check: `newIntegrationHTTPClient`, `startRotation`, `sendSamplePurchase` are real helpers written against the actual running `make up` stack's HTTP surface — implement them following whatever existing `//go:build integration` helpers `gateway-go` already has (grep for one before writing new helpers from scratch).

Run (against `make up`): `go test -tags=integration ./internal/rotation/... -run 50TPS -v` → PASS.

- [ ] **Step 4:** Wire `cmd/gateway/main.go`: construct `rotation.NewRepository(pool)`, `rotation.NewRunner(rotationRepo, keyStoreRepo, hsmModule, muxClient, zmk)` (ZMK read from a new `ZMK_HEX` env var, same `HexFormat`/`hex.DecodeString` fail-fast pattern as `LMK_TEST_VALUE_HEX`), mount `api.MountRotations(router, runnerAdapter)`.

Run: `go test ./... -race` and `make -C gateway-go lint test` clean.

- [ ] **Step 5: Commit**

```bash
git add gateway-go/internal/api/rotations.go gateway-go/internal/api/rotations_test.go gateway-go/internal/rotation/runner_integration_test.go gateway-go/cmd/gateway/main.go
git commit -m "feat(gw): POST/GET /v1/keys/acquirer/rotations, 50 TPS rotation integration test (MCN-504)"
```

- [ ] **Step 6:** Push, open PR `feat(gw): dynamic key exchange - rotation state machine (MCN-504)`.

## Self-review

- [ ] `rotation.Step.Name` values are exactly `GENERATE`/`SEND_0800_161`/`PARTNER_CONFIRM`/`ACTIVATE` — Ruling 1's contract-fixed vocabulary, matched verbatim in `MCN-504-ISS.md`.
- [ ] The clear new key generated in `GENERATE` never leaves `Runner.Run`'s local scope except as `WrapUnderLMK`'s ciphertext output and `ComputeKCV`'s hash output — confirmed by reading the implementation.
- [ ] `uq_acq_active_key`'s single-ACTIVE-row invariant is never weakened; dual-key acceptance is implemented as a bounded-window read (`FindRecentlyRetired`), not a schema change (Ruling 2).
- [ ] `dualKeyWindow` is a single named constant, referenced by both the MAC-retry and PIN-translate-retry call sites in Task 5 — no duplicated `5 * time.Minute` literal.
