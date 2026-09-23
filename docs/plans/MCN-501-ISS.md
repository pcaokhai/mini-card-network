# MCN-501-ISS Security module adapter and key store (issuer) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/iss-501`, branch `feat/MCN-501-iss-security-module`. Requires MCN-302, MCN-303 merged. Runs in parallel with **MCN-501-GW** (disjoint directories, disjoint DB schemas — issuer never reads `acquirer.key_store`). One story (8 pts) split across two lanes; see `MCN-501-GW.md` for the gateway half and Ruling 1 below for the shared representation both lanes build against independently.

**Goal:** The issuer stores every cryptographic key only as a cryptogram under an LMK-equivalent master key plus a KCV — never a clear key — and exposes `GET /v1/keys/issuer` with type/KCV/status/daysRemaining for each key. Startup fails fast if the LMK test value env var is missing.

**Architecture:** A new `application.SecurityModule` port (`generateKcv(keyUnderLmk) -> kcv`, `wrapUnderLmk(clearKeyBytes) -> keyUnderLmk`, `unwrap(keyUnderLmk) -> clearKeyBytes`) with one adapter, `JCESecurityModule`, in `adapter/crypto/` alongside the existing `CardCrypto` (same "keys under LMK, simulated with a single app-held master key from env" pattern `CardCrypto`'s own class javadoc already documents — reuse that pattern, don't invent a second one). `KeyStoreRepository` (new, `adapter/persistence/`, same shape as the existing `AccountLockRepository`/`CardLimitRepository`) reads/writes `issuer.key_store` (already defined in `docs/assets/baseline-schema.sql:275-289` — transcribe verbatim, don't redesign, same convention MCN-401's Ruling 1 established). `GET /v1/keys/issuer` is a new Javalin route on the existing `HttpEndpoints` QBean (`adapter/http/HttpEndpoints.java`, MCN-005) — confirm its current route-registration shape before adding one.

**Tech Stack:** Java 25, jPOS Q2, `javax.crypto` (AES-GCM, matching `CardCrypto`'s existing algorithm choice), Javalin, JUnit 5, AssertJ, ArchUnit.

**Spec:** `docs/06-user-stories.md` MCN-501-AC1/AC2/AC3, `docs/assets/baseline-schema.sql:275-289` (`issuer.key_store` exact DDL, transcribed below), `contracts/openapi.yaml:395-399` (`GET /v1/keys/issuer`) and `:778-796` (`KeyInfo` schema — `keyType`, `counterparty`, `kcv` pattern `^[0-9A-F]{6}$`, `status`, `activatedAt`, `daysRemaining`, `lifetimeDays`), root `CLAUDE.md` §6 rule 2 (PCI DSS — never log/persist/return clear key material), `docs/10-engineering-standards.md`.

## Global Constraints

- No clear key material is ever logged, returned by an API, or held in a Java `String` longer than the scope of one `wrapUnderLmk`/`unwrap` call — `byte[]` only, zeroed after use where the JCE API allows it (`Arrays.fill`).
- `key_store.key_under_lmk` is `TEXT` (a cryptogram, base64 or hex — pick one, document in Ruling 2) — never the clear key.
- `GET /v1/keys/issuer`'s response is exactly `contracts/openapi.yaml`'s `KeyInfo[]` shape: `kcv` only (6 hex chars), never `key_under_lmk`.
- LMK test value comes from env (`LMK_TEST_VALUE_HEX`); missing it fails startup per root `CLAUDE.md` §6 rule 11 ("fail fast on invalid config") — same pattern `CardCrypto`'s constructor already requires its two hex env keys.

## Ruling 1: KCV/key-store representation — fixed once here, `MCN-501-GW.md` must match exactly

Both lanes build in parallel with no cross-checking until integration (per the dispatch brief). The representation is **not invented per-lane** — it's already fixed by `docs/assets/baseline-schema.sql`, which independently defines both `issuer.key_store` (lines 275-289) and `acquirer.key_store` (lines 474-488) with the *same* column shape: `key_type TEXT`, an owner/counterparty column (`counterparty` on issuer, `owner_ref` on gateway — different names, same purpose, per the baseline schema's own Vietnamese comments), `key_under_lmk TEXT NOT NULL` (a cryptogram, never clear), `kcv CHAR(6) NOT NULL`, `status TEXT CHECK (status IN ('PENDING','ACTIVE','RETIRED'))`, `activated_at`/`retired_at TIMESTAMPTZ`. **KCV algorithm**: this plan computes KCV as the first 3 bytes (6 hex chars) of `AES-ECB(key=<the clear key>, plaintext=16 zero bytes)` — the standard KCV convention (ISO 9564/13491 uses "encrypt a block of zeros, take the leading bytes"), computed once in `JCESecurityModule.generateKcv` before the clear key is wrapped and discarded. `MCN-501-GW.md`'s `hsm` simulator must compute KCV identically (same zero-block-encrypt convention) so a key generated on one side and a key generated on the other produce comparably-shaped KCVs even though they are never compared directly (each side only ever computes a KCV over its *own* key, per `uq_active_key`'s per-schema uniqueness — issuer and gateway never share a literal key value in this story). Cost if this Ruling is ignored: a KCV algorithm mismatch would only surface as "the two sides' displayed KCVs look structurally different" in the WEB Security screen (MCN-505, Sprint 7) — no test in *this* sprint would catch it, so getting it right now avoids a Sprint 7 rework.

## Ruling 2: `key_under_lmk` encoding is hex, matching `kcv`'s own `CHAR(6)` hex convention

`key_store.kcv` is already `CHAR(6)` hex in the baseline DDL. `key_under_lmk` (`TEXT`) has no baseline-fixed encoding, so this plan picks hex (not base64) for consistency with `kcv` and with `CardCrypto`'s own `HexFormat.of()` usage elsewhere in `adapter/crypto/` — one encoding convention for all key-adjacent bytes-as-text in this package.

## File map

| Action | Path (under `issuer-jpos/`) |
| --- | --- |
| Create | `src/main/resources/db/migration/V6__key_store.sql` |
| Create | `src/main/java/io/mcn/issuer/application/SecurityModule.java` (port interface) |
| Create | `src/main/java/io/mcn/issuer/adapter/crypto/JCESecurityModule.java`, test |
| Create | `src/main/java/io/mcn/issuer/adapter/persistence/KeyStoreRepository.java`, `KeyStoreRow.java`, test |
| Modify | `src/main/java/io/mcn/issuer/adapter/http/HttpEndpoints.java` (add `GET /health/../keys/issuer` route — confirm actual mount point in Task 4) |
| Create | `src/test/java/io/mcn/issuer/adapter/http/KeysEndpointTest.java` |
| Modify | `src/dist/deploy/40_http_endpoints.xml` if the LMK env var needs a new `<property>` — confirm in Task 1 |

---

### Task 1: `V6__key_store.sql` migration

**Files:** `src/main/resources/db/migration/V6__key_store.sql`

- [ ] **Step 1:** Transcribe `issuer.key_store` from `docs/assets/baseline-schema.sql:275-289`, schema-prefix stripped, matching the existing `V1`-`V5` migration convention (plain Flyway SQL, no `-- +goose` markers — issuer uses Flyway, gateway uses goose; confirm by reading `V5__reversal_without_original.sql`'s header before writing this file).

```sql
CREATE TABLE key_store (
  id            BIGSERIAL PRIMARY KEY,
  key_type      TEXT NOT NULL CHECK (key_type IN ('ZMK','ZPK','ZAK','CVK','PVK','IMK_AC')),
  counterparty  VARCHAR(11),
  key_under_lmk TEXT NOT NULL,
  kcv           CHAR(6) NOT NULL,
  status        TEXT NOT NULL CHECK (status IN ('PENDING','ACTIVE','RETIRED')),
  activated_at  TIMESTAMPTZ,
  retired_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_active_key
  ON key_store(key_type, COALESCE(counterparty, ''))
  WHERE status = 'ACTIVE';
```

- [ ] **Step 2: Commit**

```bash
git add issuer-jpos/src/main/resources/db/migration/V6__key_store.sql
git commit -m "feat(iss): key_store table, transcribed from baseline schema (MCN-501)"
```

---

### Task 2: `SecurityModule` port + `JCESecurityModule` (AC1, AC3)

**Files:** `application/SecurityModule.java`, `adapter/crypto/JCESecurityModule.java`, test

**Interfaces:** `SecurityModule{ byte[] wrapUnderLmk(byte[] clearKey); byte[] unwrap(byte[] keyUnderLmk); String computeKcv(byte[] clearKey); }` (all `byte[]` in/out, no `String` clear-key parameter ever — enforces Ruling 2's "no clear key as String" constraint at the type level). `JCESecurityModule(String lmkHex)` — throws `IllegalArgumentException` at construction if `lmkHex` is null/blank (the fail-fast path; startup wiring in Task 4 reads `LMK_TEST_VALUE_HEX` and constructs this eagerly).

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.crypto;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.HexFormat;
import org.junit.jupiter.api.Test;

class JCESecurityModuleTest {

  private static final String LMK_HEX = "00112233445566778899aabbccddeeff00112233445566778899aabbccddee";

  @Test
  void should_wrap_and_unwrap_round_trip_without_exposing_clear_key__MCN_501_AC1() {
    JCESecurityModule module = new JCESecurityModule(LMK_HEX);
    byte[] clearKey = HexFormat.of().parseHex("0123456789abcdef0123456789abcdef");

    byte[] wrapped = module.wrapUnderLmk(clearKey);

    assertThat(wrapped).isNotEqualTo(clearKey);
    assertThat(module.unwrap(wrapped)).isEqualTo(clearKey);
  }

  @Test
  void should_compute_six_hex_char_kcv_as_leading_bytes_of_zero_block_encryption__MCN_501_AC2() {
    JCESecurityModule module = new JCESecurityModule(LMK_HEX);
    byte[] clearKey = HexFormat.of().parseHex("0123456789abcdef0123456789abcdef");

    String kcv = module.computeKcv(clearKey);

    assertThat(kcv).hasSize(6).matches("^[0-9A-F]{6}$");
  }

  @Test
  void should_fail_fast_when_lmk_missing__MCN_501_AC3() {
    assertThatThrownBy(() -> new JCESecurityModule(null))
        .isInstanceOf(IllegalArgumentException.class)
        .hasMessageContaining("LMK");
    assertThatThrownBy(() -> new JCESecurityModule(""))
        .isInstanceOf(IllegalArgumentException.class);
  }
}
```

Run: `./gradlew test --tests JCESecurityModuleTest` → fails (`JCESecurityModule` missing).

- [ ] **Step 2: Implement** `application/SecurityModule.java` (interface, three methods above) and `adapter/crypto/JCESecurityModule.java`: constructor validates `lmkHex` non-blank and parses it via `HexFormat.of().parseHex`, building an AES `SecretKeySpec` (32-byte LMK, same as `CardCrypto`'s `encryptionKey`); `wrapUnderLmk`/`unwrap` use `AES/GCM/NoPadding` (same cipher as `CardCrypto.encrypt`/`decrypt`, nonce-prefixed ciphertext); `computeKcv` encrypts a 16-byte zero block with `AES/ECB/NoPadding` under the clear key itself (not the LMK — the KCV proves knowledge of *that specific key*, per Ruling 1), takes the first 3 bytes, returns uppercase hex via `HexFormat.of().withUpperCase().formatHex(...)`.

Run: `./gradlew test --tests JCESecurityModuleTest` → PASS. Commit: `feat(iss): JCESecurityModule - wrap/unwrap under LMK, KCV (MCN-501)`.

---

### Task 3: `KeyStoreRepository` (AC1, AC2)

**Files:** `adapter/persistence/KeyStoreRepository.java`, `KeyStoreRow.java`, test

**Interfaces:** `KeyStoreRow{long id; String keyType; String counterparty; String keyUnderLmkHex; String kcv; String status; Instant activatedAt; Instant retiredAt; Instant createdAt}` (record). `KeyStoreRepository(DataSource)`; `.insert(KeyStoreRow row) -> long id` (status `PENDING`); `.activate(long id)` (sets `status = 'ACTIVE'`, `activated_at = now()`, and — inside the same statement's transaction — retires any existing `ACTIVE` row of the same `(key_type, counterparty)` pair, since `uq_active_key` allows only one); `.findAll() -> List<KeyStoreRow>`.

- [ ] **Step 1: Write the failing test** (Testcontainers Postgres, same pattern as the existing `CardLimitRepository`/`AccountLockRepository` tests — grep one for the exact `DataSource` bootstrap helper before writing this)

```java
package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import org.junit.jupiter.api.Test;

class KeyStoreRepositoryTest extends AbstractRepositoryTest {

  @Test
  void should_insert_pending_then_activate_retiring_previous_active__MCN_501_AC1() {
    KeyStoreRepository repo = new KeyStoreRepository(dataSource());

    long firstId = repo.insert(new KeyStoreRow(0, "ZPK", "970436000", "aa".repeat(16), "AABBCC", "PENDING", null, null, null));
    repo.activate(firstId);

    long secondId = repo.insert(new KeyStoreRow(0, "ZPK", "970436000", "bb".repeat(16), "DDEEFF", "PENDING", null, null, null));
    repo.activate(secondId);

    List<KeyStoreRow> all = repo.findAll();
    assertThat(all).filteredOn(r -> r.id() == firstId).extracting(KeyStoreRow::status).containsExactly("RETIRED");
    assertThat(all).filteredOn(r -> r.id() == secondId).extracting(KeyStoreRow::status).containsExactly("ACTIVE");
  }
}
```

Check: confirm the real base class name for Testcontainers-backed repository tests (grep `extends.*Test` in an existing `*RepositoryTest.java`) and adjust `extends AbstractRepositoryTest` to match.

Run: `./gradlew test --tests KeyStoreRepositoryTest` → fails (needs Docker; if unavailable in this environment, note it in the final report rather than skipping silently).

- [ ] **Step 2: Implement** `KeyStoreRepository` with plain JDBC (`PreparedStatement`, matching `CardLimitRepository`'s style): `insert` runs a single `INSERT ... RETURNING id`; `activate(id)` runs two statements in one JDBC transaction — `UPDATE key_store SET status='RETIRED', retired_at=now() WHERE key_type=(SELECT key_type FROM key_store WHERE id=$1) AND COALESCE(counterparty,'')=(SELECT COALESCE(counterparty,'') FROM key_store WHERE id=$1) AND status='ACTIVE'` then `UPDATE key_store SET status='ACTIVE', activated_at=now() WHERE id=$1`.

Run: `./gradlew test --tests KeyStoreRepositoryTest` → PASS. Commit: `feat(iss): KeyStoreRepository - insert, activate-and-retire-previous (MCN-501)`.

---

### Task 4: `GET /v1/keys/issuer` endpoint and LMK startup wiring (AC1, AC2, AC3)

**Files:** `adapter/http/HttpEndpoints.java` (extend), `src/test/java/io/mcn/issuer/adapter/http/KeysEndpointTest.java`, `src/dist/deploy/40_http_endpoints.xml` (extend if needed)

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

class KeysEndpointTest {
  private final HttpClient client = HttpClient.newHttpClient();
  // fakeKeyStore returns a fixed KeyStoreRow list; see Task 4 Step 2 for the exact wiring shape.

  @AfterEach
  void stop() { /* server.stop() once wired, matching HttpEndpointsTest's pattern */ }

  @Test
  void should_return_key_info_array_with_no_clear_key_field__MCN_501_AC1_AC2() throws Exception {
    // server started with a fake KeyStoreRepository returning one ACTIVE ZPK row
    HttpResponse<String> resp = client.send(
        HttpRequest.newBuilder(URI.create("http://127.0.0.1:PORT/v1/keys/issuer")).build(),
        HttpResponse.BodyHandlers.ofString());

    assertThat(resp.statusCode()).isEqualTo(200);
    assertThat(resp.body()).contains("\"kcv\"").contains("\"daysRemaining\"").doesNotContain("keyUnderLmk").doesNotContain("key_under_lmk");
  }
}
```

Check: wire this test against `HealthServer`'s real `start(int port)` pattern (`HttpEndpointsTest.java`, MCN-005) so `PORT` is the value `start(0)` returns — replace the `PORT` placeholder with the actual returned port before running; a placeholder string in test source is not acceptable in the merged commit.

Run: `./gradlew test --tests KeysEndpointTest` → fails.

- [ ] **Step 2: Implement**: add a `GET /v1/keys/issuer` route to `HealthServer`/`HttpEndpoints` (whichever currently owns route registration — confirm by reading the file; MCN-005's `HealthServer` only had `/health/*`, so this may need a new small `KeysServer` class following the exact same `Javalin.create().get(...).start(port)` shape, then registered as a route on the same Javalin app instance `HealthServer` already owns rather than a second port — reuse the existing 8081 admin port). Maps each `KeyStoreRow` to `contracts/openapi.yaml`'s `KeyInfo` JSON shape: `keyType`, `counterparty`, `kcv`, `status`, `activatedAt` (ISO-8601 or `null`), `daysRemaining` (computed as `lifetimeDays - daysSince(activatedAt)`, clamped at 0 for a `PENDING` row with no `activatedAt`), `lifetimeDays` (a fixed constant, `365`, until a rotation story like MCN-504 makes it configurable — flag with a `ponytail:` comment naming that ceiling).
- [ ] **Step 3:** Wire `cmd`/`Q2` startup: read `LMK_TEST_VALUE_HEX` from env in the same startup path that already validates other required env vars (grep `System.getenv` in an existing QBean's `startService()`); construct `new JCESecurityModule(lmkHex)` eagerly there so a missing/blank value throws `IllegalArgumentException` and Q2 fails to start (matches root `CLAUDE.md` §6 rule 11's "fail fast on invalid config").
- [ ] **Step 4:** Add `<property name="lmk-test-value" value="${env:LMK_TEST_VALUE_HEX}"/>` to whichever deploy descriptor owns the QBean wiring `JCESecurityModule` (confirm exact descriptor and env-substitution syntax against MCN-005's own "Check" note on `${env:NAME:default}` vs `$env{NAME}`, since the resolved jPOS version's syntax was only confirmed during that story's execution — reuse whatever it settled on, don't re-guess).

Run: `./gradlew test -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/http/ issuer-jpos/src/test/java/io/mcn/issuer/adapter/http/KeysEndpointTest.java issuer-jpos/src/dist/deploy/40_http_endpoints.xml
git commit -m "feat(iss): GET /v1/keys/issuer, LMK fail-fast startup (MCN-501)"
```

---

### Task 5: PCI masking test and full verification

**Files:** `src/test/java/io/mcn/issuer/adapter/crypto/JCESecurityModuleTest.java` (extend)

- [ ] **Step 1: Write the failing masking test** (PCI DSS rule 2 requires this with the same rigor as PAN masking, per root `CLAUDE.md` §6 rule 2 and §7)

```java
  @Test
  void should_never_render_clear_key_bytes_in_toString_or_exception_messages__MCN_501_AC1() {
    JCESecurityModule module = new JCESecurityModule(LMK_HEX);
    byte[] clearKey = HexFormat.of().parseHex("deadbeefdeadbeefdeadbeefdeadbeef");
    String clearKeyHex = "deadbeefdeadbeefdeadbeefdeadbeef";

    byte[] wrapped = module.wrapUnderLmk(clearKey);
    String kcv = module.computeKcv(clearKey);

    assertThat(wrapped.toString()).doesNotContain(clearKeyHex);
    assertThat(kcv).doesNotContain(clearKeyHex);
    assertThatThrownBy(() -> module.unwrap(new byte[]{1, 2, 3}))
        .isInstanceOf(RuntimeException.class)
        .hasMessageNotContaining(clearKeyHex);
  }
```

Run: `./gradlew test --tests JCESecurityModuleTest` → PASS (`JCESecurityModule.unwrap`'s exception path, implemented in Task 2, never interpolates raw bytes into its message — no code change needed here, only the test).

- [ ] **Step 2:** Commit.

```bash
git add issuer-jpos/src/test/java/io/mcn/issuer/adapter/crypto/JCESecurityModuleTest.java
git commit -m "test(iss): never-logs-clear-key masking test for JCESecurityModule (MCN-501)"
```

- [ ] **Step 3:** `./gradlew test spotlessCheck` clean. AC → test table:

| AC | Test(s) |
| --- | --- |
| MCN-501-AC1 (issuer half) | `should_wrap_and_unwrap_round_trip_without_exposing_clear_key`, `KeyStoreRepositoryTest`, `should_never_render_clear_key_bytes_in_toString_or_exception_messages` |
| MCN-501-AC2 (issuer half) | `should_compute_six_hex_char_kcv...`, `should_return_key_info_array_with_no_clear_key_field` |
| MCN-501-AC3 (issuer half) | `should_fail_fast_when_lmk_missing` |

- [ ] **Step 4:** Push, open PR `feat(iss): security module adapter and key store (MCN-501)`.

## Self-review

- [ ] `SecurityModule`'s port signature takes only `byte[]`, never `String`, for clear key material — a `String`-typed clear key would linger in the JVM string pool unmaskable.
- [ ] `GET /v1/keys/issuer` response verified byte-for-byte against `contracts/openapi.yaml`'s `KeyInfo` schema (no extra `keyUnderLmk`/`key_under_lmk` leak).
- [ ] Ruling 1's KCV convention (zero-block AES-ECB encrypt, leading 3 bytes) is the one both this plan and `MCN-501-GW.md` state — confirmed identical wording in both files before finishing.
