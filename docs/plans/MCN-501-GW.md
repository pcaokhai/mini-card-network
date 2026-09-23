# MCN-501-GW Security module adapter and key store (gateway) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/gw-501`, branch `feat/MCN-501-gw-hsm-simulator`. Requires MCN-302, MCN-303 merged. Runs in parallel with **MCN-501-ISS** (disjoint directories, disjoint DB schemas). One story (8 pts) split across two lanes; see `MCN-501-ISS.md` for the issuer half and this plan's Ruling 1 for the shared KCV representation both lanes build against independently, fixed once (not re-derived here — see `MCN-501-ISS.md`'s Ruling 1 for the full reasoning).

**Goal:** The gateway stores every cryptographic key (`TPK`, `TAK`, `ZPK`, `ZAK`) only as a cryptogram plus a KCV — never a clear key — behind a new `internal/hsm` package that will do PIN translate (TPK→ZPK) and MAC (ZAK) in MCN-502. Exposes `GET /v1/keys/acquirer` with type/KCV/status/daysRemaining. Startup fails fast if the LMK test value env var is missing.

**Architecture:** `internal/hsm.Module` mirrors the issuer's `SecurityModule` port shape (see `MCN-501-ISS.md`'s Ruling 1) — `WrapUnderLMK([]byte) ([]byte, error)`, `Unwrap([]byte) ([]byte, error)`, `ComputeKCV([]byte) (string, error)` — implemented by `hsm.JCEModule` (Go's `crypto/aes` in GCM mode for wrap/unwrap, matching `internal/store.EncryptBytes`'s existing AES-GCM convention exactly rather than inventing a second one — see Ruling 2) using the same LMK-simulation pattern the issuer uses (one app-held master key from env plays the role of an HSM's LMK). `internal/store.KeyStoreRepository` (new, alongside `SafRepository`/`TranLogRepository`) reads/writes `acquirer.key_store` (already defined in `docs/assets/baseline-schema.sql:474-488` — transcribe verbatim). `GET /v1/keys/acquirer` is a new chi route in `internal/api`, same `MountX(r chi.Router, svc Port)` pattern as `internal/api/purchases.go`'s `MountPurchases`. PIN translate and MAC themselves (the actual crypto operations MCN-502 needs) are **out of scope for this plan** — MCN-502 depends on `MCN-501-GW` only for the key storage/KCV/wrap-unwrap primitives; the `hsm` package's translate/MAC methods are added by MCN-502, not here (confirmed against `docs/06-user-stories.md`'s MCN-501 AC1, which scopes MCN-501 to "`hsm` simulator implementing PIN translate and MAC" as the *target shape* of the package, while MCN-502's own AC1/AC2 are the stories that actually add those two operations — this plan builds the package and its key-storage backbone so MCN-502 has something to extend).

**Tech Stack:** Go 1.24, `pgx`, `crypto/aes`, chi, existing `internal/store`, `internal/api` conventions.

**Spec:** `docs/06-user-stories.md` MCN-501-AC1/AC2/AC3, `docs/assets/baseline-schema.sql:474-488` (`acquirer.key_store` exact DDL, transcribed below), `contracts/openapi.yaml:401-405` (`GET /v1/keys/acquirer`) and `:778-796` (`KeyInfo` schema), root `CLAUDE.md` §6 rule 2 (PCI DSS), `MCN-501-ISS.md`'s Ruling 1 (shared KCV convention).

## Global Constraints

- No clear key material is ever logged, returned by an API, or held as a Go `string` — `[]byte` only, per Global Constraints in `MCN-501-ISS.md`.
- `key_store.key_under_lmk` is `TEXT`, hex-encoded, matching `MCN-501-ISS.md`'s Ruling 2 encoding choice exactly (one encoding convention across both lanes, even though each lane's DB is separate — consistency here matters only for a human comparing the two schemas side by side, but costs nothing to keep aligned).
- `GET /v1/keys/acquirer`'s response is exactly `contracts/openapi.yaml`'s `KeyInfo[]` shape: `kcv` only, never `keyUnderLmk`.
- LMK test value from env (`LMK_TEST_VALUE_HEX`, same env var name as the issuer side — both sides read the same-named var from their own process env, never share the literal value across services); missing it fails startup, same as `internal/config.Load`'s existing fail-fast pattern (MCN-005's `SHUTDOWN_TIMEOUT` validation is the precedent).

## Ruling 1: KCV convention — defined in `MCN-501-ISS.md`, restated here verbatim for the implementer who reads only this file

KCV = first 3 bytes (6 uppercase hex chars) of `AES-ECB(key=<the clear key itself>, plaintext=16 zero bytes)`, computed before the clear key is wrapped under the LMK and discarded. This gateway-side `hsm.JCEModule.ComputeKCV` must produce byte-for-byte the same KCV a real HSM (or the issuer's `JCESecurityModule.computeKcv`) would produce for the *same* clear key — verified by a cross-implementation test in Task 3 using a fixed known clear key and asserting the KCV matches a hand-computed expected value (not by calling into the issuer's Java code, which is a separate process/language — the two sides never literally share a key in this story, so the real proof is "both implementations follow the documented algorithm identically," checked by each side's own unit test against the same documented expected output).

## Ruling 2: reuse `store.EncryptBytes`/`DecryptBytes`, don't add a second AES-GCM helper

`internal/store/crypto.go` already implements `EncryptBytes(key, plaintext) ([]byte, error)` / `DecryptBytes(key, ciphertext) ([]byte, error)` as AES-256-GCM with a nonce-prefixed ciphertext (used today for `saf_queue.payload_enc`, MCN-401). `hsm.JCEModule.WrapUnderLMK`/`Unwrap` call these directly rather than reimplementing AES-GCM a second time in `internal/hsm` — confirmed by reading `crypto.go` before writing this plan: the function signatures already match what `hsm.Module`'s port needs (`[]byte` in, `[]byte` out, `error`), so `JCEModule` is a thin wrapper that supplies the LMK as the `key` argument. This is the ladder's rung 2 (already-in-this-codebase reuse) — a second hand-rolled AES-GCM implementation here would be pure duplication.

## File map

| Action | Path (under `gateway-go/`) |
| --- | --- |
| Create | `migrations/00004_key_store.sql` |
| Create | `internal/hsm/module.go` (`Module` port), `internal/hsm/jce.go` (`JCEModule`), tests |
| Create | `internal/store/keystore.go` (`KeyStoreRepository`), test |
| Create | `internal/api/keys.go` (`MountKeys`), test |
| Modify | `internal/config/config.go` (`LMKTestValueHex` field) |
| Modify | `cmd/gateway/main.go` (wire `hsm.JCEModule`, `KeyStoreRepository`, mount `/v1/keys/acquirer`) |

---

### Task 1: `migrations/00004_key_store.sql`

**Files:** `migrations/00004_key_store.sql`

- [ ] **Step 1:** Transcribe `acquirer.key_store` from `docs/assets/baseline-schema.sql:474-488`, schema-prefix stripped, matching the goose migration convention (`00001`-`00003` already established).

```sql
-- +goose Up
CREATE TABLE key_store (
    id            BIGSERIAL PRIMARY KEY,
    key_type      TEXT NOT NULL CHECK (key_type IN ('ZMK','ZPK','ZAK','TPK','TAK')),
    owner_ref     TEXT,
    key_under_lmk TEXT NOT NULL,
    kcv           CHAR(6) NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('PENDING','ACTIVE','RETIRED')),
    activated_at  TIMESTAMPTZ,
    retired_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_acq_active_key
    ON key_store(key_type, COALESCE(owner_ref, ''))
    WHERE status = 'ACTIVE';

-- +goose Down
DROP TABLE key_store;
```

- [ ] **Step 2: Commit**

```bash
git add gateway-go/migrations/00004_key_store.sql
git commit -m "feat(gw): key_store table, transcribed from baseline schema (MCN-501)"
```

---

### Task 2: `hsm.Module` port + `JCEModule` (AC1, AC3)

**Files:** `internal/hsm/module.go`, `internal/hsm/jce.go`, test

**Interfaces:** `Module interface{ WrapUnderLMK(clearKey []byte) ([]byte, error); Unwrap(keyUnderLMK []byte) ([]byte, error); ComputeKCV(clearKey []byte) (string, error) }`; `NewJCEModule(lmkHex string) (*JCEModule, error)` (returns an error — not a panic — when `lmkHex` is empty or not valid hex, so callers, including `config.Load`'s fail-fast wiring in Task 5, get a typed error to wrap).

- [ ] **Step 1: Write the failing test**

```go
package hsm

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

const testLMKHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddee"

func TestNewJCEModule_rejectsEmptyOrInvalidLMK__MCN_501_AC3(t *testing.T) {
	_, err := NewJCEModule("")
	require.ErrorContains(t, err, "LMK")

	_, err = NewJCEModule("not-hex")
	require.Error(t, err)
}

func TestJCEModule_wrapUnwrapRoundTrip__MCN_501_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef")

	wrapped, err := m.WrapUnderLMK(clearKey)
	require.NoError(t, err)
	require.NotEqual(t, clearKey, wrapped)

	unwrapped, err := m.Unwrap(wrapped)
	require.NoError(t, err)
	require.Equal(t, clearKey, unwrapped)
}

func TestJCEModule_computeKCV_sixHexChars__MCN_501_AC2(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef")

	kcv, err := m.ComputeKCV(clearKey)
	require.NoError(t, err)
	require.Len(t, kcv, 6)
	require.Regexp(t, "^[0-9A-F]{6}$", kcv)
}

// TestJCEModule_computeKCV_knownVector proves this implementation follows the same
// zero-block-AES-ECB-encrypt convention MCN-501-ISS.md's Ruling 1 documents, using a
// hand-computed expected value so the issuer side (separate process/language) can be checked
// against the same documented algorithm without a cross-process call.
func TestJCEModule_computeKCV_knownVector__MCN_501_AC2(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("404142434445464748494a4b4c4d4e4")

	kcv, err := m.ComputeKCV(clearKey)
	require.NoError(t, err)
	require.Equal(t, expectedKCVForTestKey, kcv) // computed once via `openssl enc -aes-128-ecb -K <clearKeyHex> -nopad` over 16 zero bytes, first 3 bytes uppercased hex
}
```

Check: `expectedKCVForTestKey` is a real constant, not a placeholder — compute it once by running `openssl enc -aes-128-ecb -K 404142434445464748494a4b4c4d4e4 -nopad -in /dev/zero -out - 2>/dev/null | head -c16 | xxd -p | tr 'a-f' 'A-F' | cut -c1-6` (or the Go implementation itself, run once and the output pasted back into the test as a literal) during Task 2 Step 2, before this test is committed — a plan cannot hand-compute a real AES block cipher output, so the implementer runs the cipher once locally and pins the result.

Run: `go test ./internal/hsm/... -run TestJCEModule` → fails (`hsm` package doesn't exist).

- [ ] **Step 2: Implement** `module.go` (the `Module` interface) and `jce.go`: `NewJCEModule` validates `lmkHex` via `hex.DecodeString`, returning `fmt.Errorf("hsm: LMK is required")` on empty and the decode error on invalid hex; stores the decoded 32-byte LMK. `WrapUnderLMK`/`Unwrap` call `store.EncryptBytes(lmk, clearKey)` / `store.DecryptBytes(lmk, wrapped)` directly (Ruling 2 — `internal/hsm` imports `internal/store` for this, a one-directional dependency that doesn't create a cycle since `store` never imports `hsm`). `ComputeKCV` builds an `aes.NewCipher(clearKey)` block cipher, encrypts a 16-byte zero block directly with `block.Encrypt` (ECB — a single block needs no block-chaining mode), takes the first 3 bytes, returns `strings.ToUpper(hex.EncodeToString(...))`. Compute `expectedKCVForTestKey` once by running this implementation against the test's known clear key and paste the real result into the test.

Run: `go test ./internal/hsm/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/hsm/module.go gateway-go/internal/hsm/jce.go gateway-go/internal/hsm/jce_test.go
git commit -m "feat(gw): hsm.JCEModule - wrap/unwrap under LMK reusing store.EncryptBytes, KCV (MCN-501)"
```

---

### Task 3: PCI masking test for `hsm.JCEModule`

**Files:** `internal/hsm/jce_test.go` (extend)

- [ ] **Step 1: Write the failing test**

```go
func TestJCEModule_neverLeaksClearKeyInErrorsOrWrappedOutput__MCN_501_AC1(t *testing.T) {
	m, err := NewJCEModule(testLMKHex)
	require.NoError(t, err)
	clearKey, _ := hex.DecodeString("deadbeefdeadbeefdeadbeefdeadbeef")
	clearKeyHex := "deadbeefdeadbeefdeadbeefdeadbeef"

	wrapped, err := m.WrapUnderLMK(clearKey)
	require.NoError(t, err)
	require.NotContains(t, hex.EncodeToString(wrapped), clearKeyHex)

	_, err = m.Unwrap([]byte{1, 2, 3})
	require.Error(t, err)
	require.NotContains(t, err.Error(), clearKeyHex)
}
```

Run: `go test ./internal/hsm/... -run TestJCEModule_neverLeaks` → fails until confirmed passing against Task 2's implementation (no production code change expected — `store.DecryptBytes`'s error path already returns a generic `"cipher: message authentication failed"`-class error with no key interpolation, confirmed by reading `crypto.go`).

Run: `go test ./internal/hsm/... -v` → PASS. Commit: `test(gw): never-logs-clear-key masking test for hsm.JCEModule (MCN-501)`.

---

### Task 4: `store.KeyStoreRepository` and `GET /v1/keys/acquirer` (AC1, AC2)

**Files:** `internal/store/keystore.go`, test; `internal/api/keys.go`, test

**Interfaces:** `KeyRow{ID int64; KeyType string; OwnerRef string; KeyUnderLMKHex string; KCV string; Status string; ActivatedAt, RetiredAt *time.Time; CreatedAt time.Time}`. `NewKeyStoreRepository(pool *Pool) *KeyStoreRepository`; `.Insert(ctx, row KeyRow) (int64, error)` (status `PENDING`); `.Activate(ctx, id int64) error` (retires the prior `ACTIVE` row of the same `(key_type, owner_ref)` pair, same two-statement-one-transaction pattern as the issuer's `KeyStoreRepository.activate`); `.List(ctx) ([]KeyRow, error)`. `MountKeys(r chi.Router, svc KeyLister)` where `KeyLister interface{ ListAcquirerKeys(ctx) ([]store.KeyRow, error) }`.

- [ ] **Step 1: Write the failing tests**

```go
package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyStoreRepository_insertThenActivateRetiresPrevious__MCN_501_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewKeyStoreRepository(pool)
	ctx := context.Background()

	firstID, err := repo.Insert(ctx, KeyRow{KeyType: "ZPK", OwnerRef: "gw-link-01", KeyUnderLMKHex: "aa", KCV: "AABBCC", Status: "PENDING"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, firstID))

	secondID, err := repo.Insert(ctx, KeyRow{KeyType: "ZPK", OwnerRef: "gw-link-01", KeyUnderLMKHex: "bb", KCV: "DDEEFF", Status: "PENDING"})
	require.NoError(t, err)
	require.NoError(t, repo.Activate(ctx, secondID))

	rows, err := repo.List(ctx)
	require.NoError(t, err)
	statuses := map[int64]string{}
	for _, r := range rows {
		statuses[r.ID] = r.Status
	}
	require.Equal(t, "RETIRED", statuses[firstID])
	require.Equal(t, "ACTIVE", statuses[secondID])
}
```

```go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeKeyLister struct{ rows []store.KeyRow }

func (f *fakeKeyLister) ListAcquirerKeys(context.Context) ([]store.KeyRow, error) { return f.rows, nil }

func TestGetKeysAcquirer_returnsKeyInfoWithNoClearKeyField__MCN_501_AC1_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountKeys(r, &fakeKeyLister{rows: []store.KeyRow{{KeyType: "ZPK", KCV: "AABBCC", Status: "ACTIVE"}}})

	req := httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"kcv":"AABBCC"`)
	require.NotContains(t, rec.Body.String(), "keyUnderLmk")
	require.NotContains(t, rec.Body.String(), "KeyUnderLMKHex")
}
```

Run: `go test ./internal/store/... ./internal/api/... -run "KeyStore|Keys"` → fails.

- [ ] **Step 2: Implement** `keystore.go` with plain `pgx` queries (same style as `safqueue.go`): `Insert` a single `INSERT ... RETURNING id`; `Activate` a two-statement transaction via `pool.BeginTx`, matching MCN-501-ISS's repository shape. `keys.go`: `MountKeys` registers `GET /v1/keys/acquirer` calling `svc.ListAcquirerKeys`, maps each `store.KeyRow` to `contracts/openapi.yaml`'s `KeyInfo` JSON (`keyType`, `counterparty: owner_ref` — note the OpenAPI field is named `counterparty` even on the acquirer side per the shared `KeyInfo` schema, so map `OwnerRef` to the JSON key `counterparty`, `kcv`, `status`, `activatedAt`, `daysRemaining` computed the same way as the issuer side (`lifetimeDays - daysSince(activatedAt)`, clamped at 0), `lifetimeDays: 365` fixed constant (same `ponytail:` ceiling comment as the issuer side — see `MCN-501-ISS.md` Task 4 Step 2).

Run: `go test ./internal/store/... ./internal/api/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/store/keystore.go gateway-go/internal/store/keystore_test.go gateway-go/internal/api/keys.go gateway-go/internal/api/keys_test.go
git commit -m "feat(gw): KeyStoreRepository, GET /v1/keys/acquirer (MCN-501)"
```

---

### Task 5: LMK config, wiring, and full verification (AC3)

**Files:** `internal/config/config.go` (extend), `internal/config/config_test.go` (extend), `cmd/gateway/main.go` (extend)

- [ ] **Step 1: Write the failing test** (extends MCN-005's `config_test.go`)

```go
func TestLoad_requiresLMKTestValueHex__MCN_501_AC3(t *testing.T) {
	_, err := Load(env(nil))
	require.ErrorContains(t, err, "LMK_TEST_VALUE_HEX")

	cfg, err := Load(env(map[string]string{"LMK_TEST_VALUE_HEX": "00112233445566778899aabbccddeeff00112233445566778899aabbccddee"}))
	require.NoError(t, err)
	require.NotEmpty(t, cfg.LMKTestValueHex)
}
```

Run: `go test ./internal/config/... -run TestLoad_requiresLMK` → fails (currently `Load` has no required fields — every other config value defaults; this is the first fail-fast-required one, so `Load`'s signature doesn't change, only its validation body gains a new branch).

- [ ] **Step 2: Implement**: add `LMKTestValueHex string` to `Config`; `Load` returns `errors.New("LMK_TEST_VALUE_HEX is required")` when `getenv("LMK_TEST_VALUE_HEX") == ""`.

Run: `go test ./internal/config/... -v` → PASS.

- [ ] **Step 3:** Wire `cmd/gateway/main.go`: construct `hsmModule, err := hsm.NewJCEModule(cfg.LMKTestValueHex)` right after `config.Load` (before any server starts — a bad LMK must fail the process before it binds a port), `keyStoreRepo := store.NewKeyStoreRepository(pool)`, mount `api.MountKeys(router, keyStoreAdapter)` where `keyStoreAdapter` wraps `keyStoreRepo.List` to satisfy `api.KeyLister`.

Run: `go test ./... -race` → PASS.

- [ ] **Step 4: Commit**

```bash
git add gateway-go/internal/config/config.go gateway-go/internal/config/config_test.go gateway-go/cmd/gateway/main.go
git commit -m "feat(gw): LMK_TEST_VALUE_HEX fail-fast config, wire hsm/KeyStoreRepository/keys route into main (MCN-501)"
```

- [ ] **Step 5:** `make -C gateway-go lint test` clean. AC → test table:

| AC | Test(s) |
| --- | --- |
| MCN-501-AC1 (gateway half) | `TestJCEModule_wrapUnwrapRoundTrip`, `TestKeyStoreRepository_insertThenActivateRetiresPrevious`, `TestJCEModule_neverLeaksClearKeyInErrorsOrWrappedOutput` |
| MCN-501-AC2 (gateway half) | `TestJCEModule_computeKCV_sixHexChars`, `TestJCEModule_computeKCV_knownVector`, `TestGetKeysAcquirer_returnsKeyInfoWithNoClearKeyField` |
| MCN-501-AC3 (gateway half) | `TestNewJCEModule_rejectsEmptyOrInvalidLMK`, `TestLoad_requiresLMKTestValueHex` |

- [ ] **Step 6:** Push, open PR `feat(gw): hsm module adapter and key store (MCN-501)`.

## Self-review

- [ ] `hsm.Module`'s port takes only `[]byte` for clear key material, mirroring `MCN-501-ISS.md`'s `SecurityModule` port shape exactly.
- [ ] `hsm.JCEModule.WrapUnderLMK`/`Unwrap` reuse `store.EncryptBytes`/`DecryptBytes` rather than a second AES-GCM implementation (Ruling 2, verified by import graph).
- [ ] `TestJCEModule_computeKCV_knownVector`'s `expectedKCVForTestKey` is a real computed hex string in the committed test, not a placeholder — confirmed before commit.
- [ ] `GET /v1/keys/acquirer` never returns `KeyUnderLMKHex` — verified by `TestGetKeysAcquirer_returnsKeyInfoWithNoClearKeyField`'s explicit `NotContains`.
- [ ] MCN-502 has everything it needs to extend `internal/hsm` with translate/MAC operations: the `Module` interface, `JCEModule`'s LMK, and `KeyStoreRepository` for looking up the active `TPK`/`ZAK` — confirmed no rework needed before MCN-502 starts.
