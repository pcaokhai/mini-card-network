# MCN-504-ISS Dynamic key exchange (issuer) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/iss-504`, branch `feat/MCN-504-iss-key-rotation`. Requires MCN-501, MCN-503 merged (this plan extends `application.SecurityModule`/`JCESecurityModule` and `adapter/persistence/KeyStoreRepository`, both already carrying wrap/unwrap/KCV and MAC verification). Runs in parallel with **MCN-504-GW** (disjoint directories, disjoint DB schemas). One story (5 pts) split across two lanes; see `MCN-504-GW.md` for the gateway (initiator) half and this plan's Ruling 1 for the shared rotation-state vocabulary, fixed once in `MCN-504-GW.md`'s Ruling 1 and restated here for the implementer who reads only this file.

**Goal:** The issuer receives a `SEND_0800_161` key-change advice (0800, DE 70=`161`, new key in DE 48 under ZMK, key index in DE 53), decrypts and stores the new key as `PENDING`, responds 0810 RC `00` (`PARTNER_CONFIRM`), then activates the new key — retiring the previous one but continuing to accept it for 5 minutes (dual-key window). Every step writes an audit record.

**Architecture:** The issuer is the **responder** in this protocol (per `MCN-504-GW.md`'s Ruling 1 — the gateway always initiates in v1); this plan adds one new jPOS `TransactionParticipant`, `ReceiveKeyChange` (`adapter/txn/`, same shape as `VerifySecurity`/`CheckLimits`), wired into the existing 0800-handling transaction manager path (confirmed against `docs/03` §7.3's echo/network-management row — 0800/0810 already has a participant chain from MCN-201/203; this adds a DE-70-`161`-specific branch, not a new message type). On receiving DE 70=`161`: `securityModule.unwrap`s DE 48 under the issuer's own ZMK (a new key, `zmk`, read from env the same way `zak`/`zpk` already are in `VerifySecurity`), computes the KCV, inserts a `PENDING` `key_store` row via the existing `KeyStoreRepository.insert` (from MCN-501-ISS — no repository change needed, confirmed in Task 2), and returns 0810 RC `00` synchronously (this **is** `PARTNER_CONFIRM` from the issuer's perspective — no separate confirm round-trip; the gateway's `PARTNER_CONFIRM` step, in `MCN-504-GW.md`, is satisfied by receiving this same 0810). `ACTIVATE` runs immediately after the 0810 is sent (same participant, same transaction) — `KeyStoreRepository.activate` (already retires-previous, from MCN-501-ISS). Dual-key acceptance during the issuer's own 5-minute window reuses the exact `FindRecentlyRetired`-style read `MCN-504-GW.md`'s Ruling 2 introduces on the gateway side, added here to the issuer's `KeyStoreRepository` as `findRecentlyRetired`, called from `VerifySecurity`'s MAC-verify and PVV-decrypt paths on a mismatch, before failing RC 96/55.

**Tech Stack:** Java 25, jPOS Q2 (`TransactionParticipant`), `javax.crypto`, JUnit 5, AssertJ.

**Spec:** `docs/06-user-stories.md` MCN-504-AC1/AC2/AC3, `docs/03-iso8583-interface-spec.md` §7.3's "161 | Key change (new ZPK/ZAK in DE 48 under ZMK, key index in DE 53) | Issuer or acquirer" row, §9's "Reversal grace for new key (DE 70 = 161) | 5 min old key accepted | Both" timer, §11's "Key change: new double-length key as a cryptogram under ZMK in DE 48 (format in §3), key index in DE 53. The receiver activates the key after 0810 RC 00 and keeps the previous key for 5 minutes.", `MCN-504-GW.md`'s Ruling 1 (rotation-state vocabulary, restated below) and Ruling 2 (dual-key acceptance as a time-boxed read, not a schema change — the issuer mirrors this exactly), root `CLAUDE.md` §6 rule 2 (PCI DSS).

## Global Constraints

- The clear new key exists only inside `ReceiveKeyChange.prepare`'s method body (unwrapped from DE 48 under ZMK, re-wrapped under LMK, KCV'd, discarded) — never logged, never held past the method call, per `MCN-501-ISS.md`'s Global Constraints (unchanged).
- `key_store` schema and `KeyStoreRepository` are unchanged from MCN-501-ISS except for the one new read method (Ruling 2, restated from `MCN-504-GW.md`) — no new columns, no weakening of `uq_active_key`.
- Dual-key acceptance window is exactly 5 minutes (`docs/03` §9), a named constant shared with `VerifySecurity`'s existing MAC/PVV verify code, not duplicated.
- DE 48 (the key-change cryptogram) and DE 52 (PIN block) are both removed from the `Context`/`ISOMsg` immediately after use, on every code path — `ReceiveKeyChange` follows the exact `finally { request.unset(...) }` pattern `VerifySecurity` already established for DE 52 (`docs/03` §11's DE 52 boundary Ruling from `SPRINT-6.md`).

## Ruling 1: rotation-state vocabulary — defined in `MCN-504-GW.md`, restated here verbatim

`GENERATE`, `SEND_0800_161`, `PARTNER_CONFIRM`, `ACTIVATE` (`contracts/openapi.yaml`'s `KeyRotation.steps[].name` enum) are the only step names either lane uses. The issuer never runs `GENERATE` (it never originates a new key in v1) or persists its own `KeyRotation` resource (there is no `POST /v1/keys/issuer/rotations` in the contract) — it only *participates* in `SEND_0800_161` (as the receiver of the 0800) and implicitly satisfies `PARTNER_CONFIRM`/`ACTIVATE` by sending 0810 RC `00` and then activating. `ReceiveKeyChange`'s own internal audit events (Task 3) are named `key_change.received`, `key_change.confirmed`, `key_change.activated` — deliberately *not* reusing the gateway's rotation-step names verbatim for its own audit log, since the issuer's audit trail is a different resource (there is no issuer-side `KeyRotation` row to update); the four-step vocabulary is the gateway's rotation resource language, and the issuer's job is only to make each of those steps observably true on its own side (a `SEND_0800_161` the issuer received and answered `00` to, a `PARTNER_CONFIRM`-equivalent the issuer's own 0810 constitutes, an `ACTIVATE` the issuer's own `KeyStoreRepository.activate` call performs) — confirmed no code needs to import or reference `contracts/openapi.yaml`'s enum literals on the issuer side, since the issuer never serializes a `KeyRotation` JSON response.

## File map

| Action | Path (under `issuer-jpos/`) |
| --- | --- |
| Modify | `src/main/java/io/mcn/issuer/adapter/persistence/KeyStoreRepository.java` (`findRecentlyRetired`), test |
| Create | `src/main/java/io/mcn/issuer/adapter/txn/ReceiveKeyChange.java`, test |
| Modify | `src/main/java/io/mcn/issuer/adapter/txn/VerifySecurity.java` (dual-key retry on MAC/PVV mismatch), test |
| Modify | `src/dist/deploy/*.xml` (register `ReceiveKeyChange` in the 0800 transaction manager chain — confirm exact descriptor in Task 3) |

---

### Task 1: `KeyStoreRepository.findRecentlyRetired` (AC2)

**Files:** `adapter/persistence/KeyStoreRepository.java` (extend), test

**Interfaces:** add `Optional<KeyStoreRow> findRecentlyRetired(String keyType, String counterparty, Duration within)`.

- [x] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.util.Optional;
import org.junit.jupiter.api.Test;

class KeyStoreRepositoryRecentlyRetiredTest extends AbstractRepositoryTest {

  @Test
  void should_find_recently_retired_key_within_window__MCN_504_AC2() {
    KeyStoreRepository repo = new KeyStoreRepository(dataSource());

    long firstId = repo.insert(new KeyStoreRow(0, "ZAK", "970436000", "aa".repeat(16), "AAAAAA", "PENDING", null, null, null));
    repo.activate(firstId);
    long secondId = repo.insert(new KeyStoreRow(0, "ZAK", "970436000", "bb".repeat(16), "BBBBBB", "PENDING", null, null, null));
    repo.activate(secondId); // retires firstId

    Optional<KeyStoreRow> recent = repo.findRecentlyRetired("ZAK", "970436000", Duration.ofMinutes(5));
    assertThat(recent).isPresent();
    assertThat(recent.get().id()).isEqualTo(firstId);

    Optional<KeyStoreRow> stale = repo.findRecentlyRetired("ZAK", "970436000", Duration.ZERO);
    assertThat(stale).isEmpty();
  }
}
```

Run: `./gradlew test --tests KeyStoreRepositoryRecentlyRetiredTest` → fails.

- [x] **Step 2: Implement**: `SELECT * FROM key_store WHERE key_type=? AND counterparty=? AND status='RETIRED' AND retired_at > now() - ?::interval ORDER BY retired_at DESC LIMIT 1`, bind `within` as a Postgres interval string (e.g. `"5 minutes"` built from `within.toMinutes()`), map to `KeyStoreRow`, `Optional.empty()` on no rows.

Run: `./gradlew test --tests KeyStoreRepositoryRecentlyRetiredTest` → PASS. Commit: `feat(iss): KeyStoreRepository.findRecentlyRetired - dual-key acceptance window read (MCN-504)`.

---

### Task 2: `ReceiveKeyChange` participant — receive, confirm, activate (AC1, AC3)

**Files:** `adapter/txn/ReceiveKeyChange.java`, test

**Interfaces:** `ReceiveKeyChange(SecurityModule securityModule, KeyStoreRepository keyStoreRepository, byte[] zmk)`; jPOS `TransactionParticipant.prepare(long, Serializable)`. Consumes `ctx.get(TxnContextKeys.REQUEST)` (an `ISOMsg` with DE 70=`161`, DE 48=key cryptogram under ZMK, DE 53=key index) and `TxnContextKeys.ACQUIRER_ID` (the acquirer id, already available on other participants per `VerifySecurity`'s precedent — confirm exact key name in `TxnContextKeys` before use). Produces: sets `ctx.put(TxnContextKeys.RESPONSE_CODE, "00")` on success, an audit write, and (unlike other participants) directly triggers `keyStoreRepository.activate` within the same `prepare` call rather than deferring to `commit` — a rotation's activation is not something later participants need to see mid-transaction, so no new context key is introduced for it.

- [x] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import io.mcn.issuer.application.SecurityModule;
import java.util.HexFormat;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class ReceiveKeyChangeTest {

  @Test
  void should_unwrap_store_pending_confirm_then_activate__MCN_504_AC1_AC3() throws Exception {
    SecurityModule securityModule = mock(SecurityModule.class);
    KeyStoreRepository keyStoreRepository = mock(KeyStoreRepository.class);
    byte[] zmk = HexFormat.of().parseHex("00".repeat(32));
    byte[] newClearKey = HexFormat.of().parseHex("11".repeat(16));
    when(securityModule.unwrap(any(), eq(zmk))).thenReturn(newClearKey);
    when(securityModule.wrapUnderLmk(newClearKey)).thenReturn(HexFormat.of().parseHex("ff".repeat(16)));
    when(securityModule.computeKcv(newClearKey)).thenReturn("DDEEFF");
    when(keyStoreRepository.insert(any())).thenReturn(42L);

    ReceiveKeyChange participant = new ReceiveKeyChange(securityModule, keyStoreRepository, zmk);

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");
    request.set(48, HexFormat.of().formatHex(newClearKey)); // cryptogram under ZMK, simulated as hex passthrough for this fake
    request.set(53, "01");

    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.ACQUIRER_ID, "970436000");

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(TransactionParticipantResult.PREPARED); // adjust to the real ABORTED/PREPARED constant names used elsewhere in this package
    assertThat((String) ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
    verify(keyStoreRepository).insert(argThat(row -> "ZAK".equals(row.keyType()) || "ZPK".equals(row.keyType())));
    verify(keyStoreRepository).activate(42L);
    assertThat(request.hasField(48)).isFalse();
  }
}
```

Check: `TransactionParticipantResult.PREPARED` is a placeholder name — replace with the real `PREPARED`/`ABORTED` int constants `VerifySecurity`/`CheckLimits` already use (`TransactionParticipant.PREPARED`, confirmed by reading either existing participant's imports) before this test is finalized.

Run: `./gradlew test --tests ReceiveKeyChangeTest` → fails.

- [x] **Step 2: Implement**: `prepare` reads DE 70; if not `"161"`, returns `PREPARED` immediately (a no-op for every other 0800 — echo/sign-on keep their existing participants unaffected, per `docs/03` §7.3 treating `161` as one specific business-code branch of 0800, not a new MTI). If `"161"`: reads DE 48 (hex-decode), DE 53 (key index — stored for completeness though not yet used for multi-key-per-type selection), `keyType` from... (the request needs a way to say `ZPK` vs `ZAK` — since DE 48/53 alone don't carry it, add DE 70's businesscode-adjacent convention: reuse the existing acquirer-scoped `counterparty` context and a new private DE (or a fixed test convention documented here as a Ruling-worthy deviation) — **decision for this plan**: key type is carried in DE 123 (a private-use field already reserved but unused per `docs/03` §3's private-field range, confirmed unused by grepping `spec_gen`/packager files before use) as `"ZPK"` or `"ZAK"` literal; this is the smallest addition that avoids inventing a new MTI variant). `securityModule.unwrap(newKeyUnderZmk, zmk)` → clear key; `securityModule.wrapUnderLmk(clearKey)` → cryptogram under LMK; `securityModule.computeKcv(clearKey)`; `keyStoreRepository.insert(new KeyStoreRow(0, keyType, counterpartyId, hex(wrapped), kcv, "PENDING", null, null, null))` → id; set RC `00` (this response, once packed and sent as 0810 by the existing send-response path, **is** `PARTNER_CONFIRM` — no extra step); `keyStoreRepository.activate(id)`; `finally { request.unset(48); }` (DE 48 never survives past this participant, mirroring DE 52's existing boundary in `VerifySecurity`).

Run: `./gradlew test --tests ReceiveKeyChangeTest` → PASS.

- [x] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/txn/ReceiveKeyChange.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/txn/ReceiveKeyChangeTest.java
git commit -m "feat(iss): ReceiveKeyChange - receive/confirm/activate key change advice (MCN-504)"
```

---

### Task 3: Wire `ReceiveKeyChange` into the 0800 transaction chain, audit writes (AC1, AC3)

**Files:** `src/dist/deploy/*.xml` (confirm exact 0800 chain descriptor), `adapter/txn/ReceiveKeyChange.java` (extend for audit)

- [x] **Step 1:** Confirm the real 0800 participant chain descriptor (`grep -rl "0800\|echo\|NetworkManagement" issuer-jpos/src/dist/deploy/*.xml`) — the sign-on/echo participants from MCN-201/203 already register a chain; add `ReceiveKeyChange` to it, ordered after any existing DE-70 dispatch/routing participant and before the response-send participant.
- [x] **Step 2:** Extend `ReceiveKeyChange.prepare` to call an injected `AuditWriter` (or the same `dataSource`-backed `audit_log` insert `issuer.audit_log` already supports per `docs/05-data-model.md` §2's "outbox_event, audit_log | Events to Kafka; append-only audit") once per successful key change: `event_type = "key_change.activated"`, `detail` JSON `{keyType, counterparty, newKcv}` — no clear key or cryptogram in the audit detail (PCI DSS boundary, same as everywhere else in this plan).
- [x] **Step 3:** Extend `ReceiveKeyChangeTest` with an audit-write assertion (`verify(auditWriter).write(eq("key_change.activated"), any()))`.

Run: `./gradlew test --tests ReceiveKeyChangeTest` → PASS.

- [x] **Step 4: Commit**

```bash
git add issuer-jpos/src/dist/deploy/ issuer-jpos/src/main/java/io/mcn/issuer/adapter/txn/ReceiveKeyChange.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/txn/ReceiveKeyChangeTest.java
git commit -m "feat(iss): wire ReceiveKeyChange into 0800 chain, audit write on activation (MCN-504)"
```

---

### Task 4: Dual-key retry in `VerifySecurity` (AC2)

**Files:** `adapter/txn/VerifySecurity.java` (extend), test

- [x] **Step 1: Write the failing test**

```java
  @Test
  void should_verify_mac_against_recently_retired_zak_during_rotation_window__MCN_504_AC2() throws Exception {
    SecurityModule securityModule = mock(SecurityModule.class);
    CardRepository cardRepository = mock(CardRepository.class);
    KeyStoreRepository keyStoreRepository = mock(KeyStoreRepository.class);
    byte[] activeZak = "active-zak-test".getBytes();
    byte[] retiredZak = "retired-zak-test".getBytes();
    when(keyStoreRepository.findRecentlyRetired(eq("ZAK"), any(), any()))
        .thenReturn(Optional.of(new KeyStoreRow(9, "ZAK", "970436000", "aa", "AAAAAA", "RETIRED", null, null, null)));
    when(securityModule.unwrap(any(), any())).thenReturn(retiredZak);
    when(securityModule.computeMac(any(), eq(activeZak))).thenReturn(new byte[]{9, 9, 9, 9, 9, 9, 9, 9});
    when(securityModule.computeMac(any(), eq(retiredZak))).thenReturn(new byte[]{1, 2, 3, 4, 5, 6, 7, 8});

    VerifySecurity participant =
        new VerifySecurity(securityModule, cardRepository, keyStoreRepository, activeZak, new byte[16]);

    ISOMsg request = new ISOMsg("0200");
    request.set(64, new byte[]{1, 2, 3, 4, 5, 6, 7, 8}); // matches the retired key's MAC, not the active one's

    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    int result = participant.prepare(1L, ctx);

    assertThat(result).isNotEqualTo(TransactionParticipant.ABORTED);
    assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isNotEqualTo("96");
  }
```

Run: `./gradlew test --tests VerifySecurityTest` → fails.

- [x] **Step 2: Implement**: `VerifySecurity` gains a `KeyStoreRepository keyStoreRepository` field/constructor param (also wired in `setConfiguration`). In `verifyMac`, on the initial mismatch against `zak`, call `keyStoreRepository.findRecentlyRetired("ZAK", counterpartyId, DUAL_KEY_WINDOW)` (a new `private static final Duration DUAL_KEY_WINDOW = Duration.ofMinutes(5);` constant, matching `docs/03` §9's timer exactly); if present, `securityModule.unwrap` its `keyUnderLmk`, recompute the MAC, and treat a match as verified. No match (or no recently-retired row): existing RC 96 behavior unchanged. Same retry pattern applied to the PVV `zpk` lookup used in `verifyPin`.

Run: `./gradlew test --tests VerifySecurityTest` → PASS.

- [x] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/txn/VerifySecurity.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/txn/VerifySecurityTest.java
git commit -m "feat(iss): dual-key acceptance retry in VerifySecurity MAC/PVV checks (MCN-504)"
```

---

### Task 5: Full verification

**Files:** none (verification-only)

- [x] **Step 1:** `./gradlew test spotlessCheck` clean. AC → test table:

| AC | Test(s) |
| --- | --- |
| MCN-504-AC1 (issuer half) | `ReceiveKeyChangeTest` (receive, confirm via RC 00, insert PENDING then activate) |
| MCN-504-AC2 (issuer half) | `KeyStoreRepositoryRecentlyRetiredTest`, `VerifySecurityTest`'s dual-key MAC retry test |
| MCN-504-AC3 (issuer half) | `ReceiveKeyChangeTest`'s audit-write assertion (Task 3) |

- [x] **Step 2:** Push, open PR `feat(iss): key rotation - receive/confirm/activate, dual-key window (MCN-504)`.

## Self-review

- [x] `ReceiveKeyChange` never returns, logs, or persists the clear new key — verified by reading the implementation, not just the round-trip test.
- [x] DE 48 is `unset` in a `finally` block on every code path, mirroring DE 52's existing boundary in `VerifySecurity` (`SPRINT-6.md`'s DE 52 Ruling, extended here to DE 48).
- [x] Ruling 1's step-vocabulary scoping (issuer never emits a `KeyRotation` JSON, never runs `GENERATE`) is reflected in the actual code — no dead code path attempting to serialize `contracts/openapi.yaml`'s `KeyRotation` schema from the issuer side.
- [x] `DUAL_KEY_WINDOW` in `VerifySecurity` and the window used in `ReceiveKeyChange`/`findRecentlyRetired` calls are the same 5-minute value — confirmed identical to `MCN-504-GW.md`'s `dualKeyWindow` constant.
