# MCN-302a Issuer authorization chain — validate & respond — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/iss-302a-validate-respond`, branch `feat/MCN-302a-validate-respond`. Requires MCN-301 merged (schema, seed, `CardRepository`). Part 1 of 2 for story MCN-302 (13 pts total, split 302a=5/302b=8 per `docs/07-delivery-plan.md`). **302b (approval, ledger posting, concurrency/perf proof) is a separate story/plan and out of scope here** — this story wires the jPOS `TransactionManager` participant chain and every *decline* path; the approval path is intentionally stubbed (see Ruling) until 302b adds real ledger posting.

**Goal:** A `TransactionManager` participant chain (`ParseAndValidate → Deduplicate → CheckCard → VerifySecurity → CheckLimits → Authorize → LogAndOutbox → Respond`) exists and correctly declines every case the spec defines short of a real approval: format errors (RC 30), unsigned link (RC 91), duplicate requests (replay stored response), unknown card (RC 14), blocked/lost/stolen card (RC 62), expired card (RC 54), invalid amount (RC 13), and limit exceeded (RC 61). `tran_log` gets a row for every attempt, approved or not.

**Architecture:** Each box in the spec's participant chain is a jPOS `TransactionParticipant` (`prepare()`/`commit()`/`abort()`), registered in order in a new Q2 `TransactionManager` deploy descriptor, invoked from `NetworkManagementListener`'s sibling — a new `AuthorizationListener` (`ISORequestListener`) that only accepts MTI class `01`/`02` (financial) and hands the message to `TransactionManager.queue(...)` instead of jPOS's simpler direct-reply pattern MCN-201 used for network management. `VerifySecurity` is a real participant in the chain (so the ordering and wiring are correct end to end) but its `prepare()` unconditionally returns `PREPARED` — a documented no-op until E5 (MCN-501+), exactly as the user story itself says.

**Tech Stack:** Java 25, jPOS 3.0.1 `TransactionManager`/`TransactionParticipant`/`Context`, `CardRepository`/`AccountRepository` (MCN-301), Testcontainers Postgres.

**Spec:** `docs/03-iso8583-interface-spec.md` §6 (processing codes), §7.2 (auth flow), §7.5 (duplicates), §8 (RC table), `docs/05-data-model.md` (`tran_log`, `uq_tran_dedupe`), `docs/06-user-stories.md` MCN-302 (items 1–2 are this story's scope; items 3–5 are 302b's).

## Global Constraints

- RC values are exactly the ones in `docs/03` §8 — do not invent codes.
- `tran_log`'s dedupe key is `(acquirer_id, tid, stan, transmission_dt_raw, mti, business_date)` per `docs/05` §6 and the `uq_tran_dedupe` constraint from MCN-301's migration; MTI x21 repeats are normalized to x20 before the dedupe check, per `docs/03` §5.
- Every request gets a `tran_log` row, approved or declined — `LogAndOutbox` runs on every path, including declines, not only approvals.
- Money stays `long`/`BIGINT` minor units throughout; no `double`/`BigDecimal` anywhere in this chain.

## Ruling: what "Authorize" means in 302a vs 302b

The spec's participant list has one box named `Authorize`. Splitting the 13-point story at the 5/8 boundary means 302a's `Authorize` participant handles every *decline* determination (unknown card, blocked/expired card, invalid amount, limit exceeded) and, on a would-be approval, sets `Context` status to `PENDING_LEDGER` and a marker `authNotYetImplemented=true` rather than fabricating a fake RC 00. `Respond` then maps `PENDING_LEDGER` to **RC 96** (system malfunction) with a clear internal `decline_reason` of `"ledger posting not implemented until MCN-302b"` — this is an honest, testable placeholder, not a silent wrong-answer RC 00. 302b replaces this branch with real ledger posting and RC 00 + DE 38; no other participant in the chain changes. This keeps 302a fully mergeable and testable on its own (every AC-2 decline code from the user story except `00`/`51` is provable now) without inventing money movement ahead of the story that actually earns it.

`RC 51` (insufficient funds) is also 302b's — checking "enough funds" requires reading `account.available_balance`, which is meaningless to check without the ledger semantics 302b introduces (holds, `chk_available_floor`); 302a's `CheckLimits` only checks `card_limit`/`velocity_counter` (RC 61/65), not account balance.

## File map

| Action | Path (under `issuer-jpos/`) |
| --- | --- |
| Create | `src/main/java/io/mcn/issuer/domain/txn/{ParseAndValidate,Deduplicate,CheckCard,VerifySecurity,CheckLimits,Authorize,LogAndOutbox,Respond}.java` + one test class per participant |
| Create | `src/main/java/io/mcn/issuer/adapter/iso/AuthorizationListener.java`, test |
| Create | `src/main/java/io/mcn/issuer/adapter/persistence/TranLogRepository.java`, test |
| Create | `src/test/java/io/mcn/issuer/PurchaseDeclineIntegrationTest.java` (real socket, real TransactionManager, real Postgres) |
| Modify | `src/dist/deploy/30_iso_server.xml` (register `AuthorizationListener` alongside `NetworkManagementListener`) |
| Create | `src/dist/deploy/40_txn_manager.xml` |

---

### Task 1: `TranLogRepository` — insert + dedupe lookup

**Files:** `adapter/persistence/TranLogRepository.java`, test

**Interfaces:** `insert(TranLogRow row) -> long id`; `findByDedupeKey(String acquirerId, String tid, String stan, String transmissionDtRaw, String mti, LocalDate businessDate) -> Optional<TranLogRow>`; `TranLogRow` is a record mirroring the columns this story actually writes (not every baseline column — only what MCN-302a populates; MCN-302b/401/etc. extend the same repository later).

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.persistence;

import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.time.LocalDate;

import static org.assertj.core.api.Assertions.assertThat;

@Testcontainers
class TranLogRepositoryTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine")
            .withDatabaseName("issuer").withUsername("issuer").withPassword("test");

    @Test
    void insertThenFindByDedupeKey() {
        var ds = TestDataSources.migrated(postgres);
        var repo = new TranLogRepository(ds);
        var row = new TranLogRow(LocalDate.of(2026, 9, 22), "0200", "PURCHASE", "000000",
                "970499", "00000042", "GOCPHO000000001", "000001", "0922120000",
                "12345678", 100_000L, "704", null, "RECEIVED", null, null);

        repo.insert(row);
        var found = repo.findByDedupeKey("970499", "00000042", "000001", "0922120000", "0200", LocalDate.of(2026, 9, 22));

        assertThat(found).isPresent();
        assertThat(found.get().rrn()).isEqualTo("12345678");
    }

    @Test
    void dedupeLookupMissesOnDifferentStan() {
        var ds = TestDataSources.migrated(postgres);
        var repo = new TranLogRepository(ds);
        assertThat(repo.findByDedupeKey("970499", "00000042", "999999", "0922120000", "0200", LocalDate.of(2026, 9, 22))).isEmpty();
    }
}
```

Run: `./gradlew test --tests TranLogRepositoryTest` → fails.

- [ ] **Step 2: Implement** `TranLogRow` (record) and `TranLogRepository` (plain JDBC, same style as MCN-301's `CardRepository`) — insert covers `business_date, mti, tran_type, processing_code, acquirer_id, tid, mid, stan, transmission_dt_raw, rrn, amount, currency, card_id, status, response_code, decline_reason`; `findByDedupeKey` selects on the exact `uq_tran_dedupe` column tuple. Normalize MTI `x21` to `x20` inside the repository call site (the caller, `Deduplicate`, does the normalization before calling — the repository just takes whatever MTI string it's given).

Run: `./gradlew test --tests TranLogRepositoryTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/persistence/TranLogRepository.java issuer-jpos/src/main/java/io/mcn/issuer/adapter/persistence/TranLogRow.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/persistence/TranLogRepositoryTest.java
git commit -m "feat(iss): TranLogRepository - insert and dedupe lookup (MCN-302a)"
```

---

### Task 2: `ParseAndValidate` and `Deduplicate` participants

**Files:** `domain/txn/{ParseAndValidate,Deduplicate}.java`, tests

**Interfaces:** Both implement `org.jpos.transaction.TransactionParticipant`. `Context` keys (constants in a shared `TxnContextKeys` class, since every later participant reads/writes the same keys): `REQUEST` (the parsed `ISOMsg`), `RESPONSE_CODE`, `DECLINE_REASON`, `AMOUNT`, `PROCESSING_CODE`, `ACQUIRER_ID`, `RRN`, `IS_DUPLICATE`, `STORED_RESPONSE` (an `ISOMsg` to replay verbatim on a duplicate hit).

- [ ] **Step 1: Write the failing tests**

```java
package io.mcn.issuer.domain.txn;

import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.jpos.transaction.TransactionConstants.PREPARED_FOR_ABORT;

class ParseAndValidateTest {

    @Test
    void invalidAmountAbortsWithRc13() throws Exception {
        ISOMsg msg = new ISOMsg("0200");
        msg.set(3, "000000");
        msg.set(4, "000000000000"); // zero amount - invalid per RC 13
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, msg);

        int result = new ParseAndValidate().prepare(1L, ctx);

        assertThat(result & PREPARED_FOR_ABORT).isNotZero();
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("13");
    }

    @Test
    void unsupportedProcessingCodeAbortsWithRc12() throws Exception {
        ISOMsg msg = new ISOMsg("0200");
        msg.set(3, "999999");
        msg.set(4, "000000010000");
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, msg);

        int result = new ParseAndValidate().prepare(1L, ctx);

        assertThat(result & PREPARED_FOR_ABORT).isNotZero();
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("12");
    }

    @Test
    void validPurchaseIsPrepared() throws Exception {
        ISOMsg msg = new ISOMsg("0200");
        msg.set(3, "000000");
        msg.set(4, "000000010000");
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, msg);

        int result = new ParseAndValidate().prepare(1L, ctx);

        assertThat(result & PREPARED).isNotZero();
        assertThat(ctx.get(TxnContextKeys.AMOUNT)).isEqualTo(10000L);
    }
}
```

```java
package io.mcn.issuer.domain.txn;

import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

import java.time.LocalDate;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.jpos.transaction.TransactionConstants.PREPARED_FOR_ABORT;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

class DeduplicateTest {

    @Test
    void firstRequestIsPrepared() {
        var repo = Mockito.mock(TranLogRepository.class);
        when(repo.findByDedupeKey(any(), any(), any(), any(), any(), any())).thenReturn(java.util.Optional.empty());
        Context ctx = freshContext();

        int result = new Deduplicate(repo).prepare(1L, ctx);

        assertThat(result & PREPARED).isNotZero();
        assertThat(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE)).isFalse();
    }

    @Test
    void repeatRequestIsMarkedDuplicateNotAborted() {
        var repo = Mockito.mock(TranLogRepository.class);
        var stored = new TranLogRow(LocalDate.now(), "0200", "PURCHASE", "000000", "970499",
                "00000042", "GOCPHO000000001", "000001", "0922120000", "12345678", 10000L, "704", null, "APPROVED", "00", null);
        when(repo.findByDedupeKey(any(), any(), any(), any(), any(), any())).thenReturn(java.util.Optional.of(stored));
        Context ctx = freshContext();

        // Duplicates are NOT aborted - they flow through to Respond, which replays the stored outcome.
        int result = new Deduplicate(repo).prepare(1L, ctx);

        assertThat(result & PREPARED).isNotZero();
        assertThat(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE)).isTrue();
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
    }

    private Context freshContext() {
        ISOMsg msg = new ISOMsg("0200");
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, msg);
        ctx.put(TxnContextKeys.ACQUIRER_ID, "970499");
        ctx.put(TxnContextKeys.PROCESSING_CODE, "000000");
        return ctx;
    }
}
```

Check: `TransactionParticipant.prepare(long id, Serializable context)` signature and the `PREPARED`/`PREPARED_FOR_ABORT`/`ABORTED` bitmask constants must be confirmed against the real jPOS 3.0.1 `TransactionConstants`/`TransactionParticipant` interface (`javap org.jpos.transaction.TransactionParticipant`) before finalizing these tests — jPOS's exact API shape here is exactly the kind of thing this project has repeatedly found different from assumption (MCN-201's `ISOServer`/`QServer` discovery is the most recent example). Adjust signatures if the real interface differs.

Run: `./gradlew test --tests ParseAndValidateTest --tests DeduplicateTest` → fail.

- [ ] **Step 2: Implement** `TxnContextKeys` (a class of `String` constants), `ParseAndValidate` (validates DE 3 is a known processing code from `docs/03` §6, DE 4 > 0 and ≤ a configured system max, sets RC 12/13 and `PREPARED_FOR_ABORT` on failure, else extracts `AMOUNT`/`PROCESSING_CODE`/`ACQUIRER_ID` into the context and returns `PREPARED`), `Deduplicate` (constructor-injects `TranLogRepository`; on a hit, sets `IS_DUPLICATE=true` plus the stored RC/response fields and still returns `PREPARED` — a duplicate is not an abort, it's a different path through `Respond`).

Run: `./gradlew test --tests ParseAndValidateTest --tests DeduplicateTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/domain/txn/{TxnContextKeys,ParseAndValidate,Deduplicate}.java issuer-jpos/src/test/java/io/mcn/issuer/domain/txn/{ParseAndValidateTest,DeduplicateTest}.java
git commit -m "feat(iss): ParseAndValidate and Deduplicate transaction participants (MCN-302a)"
```

---

### Task 3: `CheckCard`, `VerifySecurity` (no-op), `CheckLimits`

**Files:** `domain/txn/{CheckCard,VerifySecurity,CheckLimits}.java`, tests

- [ ] **Step 1: Write the failing tests** (one file per participant; illustrative excerpt for `CheckCard`, follow the same shape for the other two)

```java
package io.mcn.issuer.domain.txn;

import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.jpos.transaction.TransactionConstants.PREPARED_FOR_ABORT;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

class CheckCardTest {

    @Test
    void unknownCardAbortsWithRc14() {
        var repo = Mockito.mock(CardRepository.class);
        when(repo.findByPanHash(any())).thenReturn(java.util.Optional.empty());
        Context ctx = new Context();
        ctx.put(TxnContextKeys.PAN_HASH, new byte[]{1, 2, 3});

        int result = new CheckCard(repo).prepare(1L, ctx);

        assertThat(result & PREPARED_FOR_ABORT).isNotZero();
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("14");
    }

    @Test
    void blockedCardAbortsWithRc62() {
        var repo = Mockito.mock(CardRepository.class);
        when(repo.findByPanHash(any())).thenReturn(java.util.Optional.of(new Card(1L, 1L, "970436", "3310", "2707", "BLOCKED")));
        Context ctx = new Context();
        ctx.put(TxnContextKeys.PAN_HASH, new byte[]{1, 2, 3});

        int result = new CheckCard(repo).prepare(1L, ctx);

        assertThat(result & PREPARED_FOR_ABORT).isNotZero();
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("62");
    }

    @Test
    void expiredCardAbortsWithRc54() {
        var repo = Mockito.mock(CardRepository.class);
        when(repo.findByPanHash(any())).thenReturn(java.util.Optional.of(new Card(1L, 1L, "970436", "7765", "2001", "ACTIVE")));
        Context ctx = new Context();
        ctx.put(TxnContextKeys.PAN_HASH, new byte[]{1, 2, 3});
        ctx.put(TxnContextKeys.BUSINESS_DATE, java.time.LocalDate.of(2026, 9, 22));

        int result = new CheckCard(repo).prepare(1L, ctx);

        assertThat(result & PREPARED_FOR_ABORT).isNotZero();
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("54");
    }

    @Test
    void activeUnexpiredCardIsPrepared() {
        var repo = Mockito.mock(CardRepository.class);
        when(repo.findByPanHash(any())).thenReturn(java.util.Optional.of(new Card(1L, 1L, "970436", "4417", "2811", "ACTIVE")));
        Context ctx = new Context();
        ctx.put(TxnContextKeys.PAN_HASH, new byte[]{1, 2, 3});
        ctx.put(TxnContextKeys.BUSINESS_DATE, java.time.LocalDate.of(2026, 9, 22));

        int result = new CheckCard(repo).prepare(1L, ctx);

        assertThat(result & PREPARED).isNotZero();
        assertThat(ctx.<Long>get(TxnContextKeys.ACCOUNT_ID)).isEqualTo(1L);
    }
}
```

Write equivalent tests for `VerifySecurity` (single test: `prepareAlwaysReturnsPrepared`, asserting the no-op contract explicitly so a future E5 story that changes this behavior gets a clear, intentional test failure rather than silent drift) and `CheckLimits` (mock a limits/velocity lookup port — define a small `LimitsPort` interface here, implemented for real once `card_limit`/`velocity_counter` repositories exist; if MCN-301 didn't build those repositories, add a minimal `CardLimitRepository.findApplicableLimit(cardId, tranType) -> Optional<Limit>` in this task rather than blocking on scope creep back into MCN-301 — small, additive, needed here).

Run: `./gradlew test --tests CheckCardTest --tests VerifySecurityTest --tests CheckLimitsTest` → fail.

- [ ] **Step 2: Implement** all three participants following `CheckCard`'s pattern above: expiry check compares `expiry_yymm` (YYMM) against the transaction's business date (YYMM ≥ current, per `docs/03`'s DE 14 semantics — a card is expired once the current business-date's YYMM exceeds it); `CheckLimits` compares `AMOUNT` against any matching `PER_TXN`/`DAILY` limit row and aborts RC 61 if exceeded, RC 65 if a count-based limit is exceeded (`docs/03` §8 distinguishes amount-limit 61 from frequency-limit 65 — implement both branches even though the user story text only calls out 61, since both codes already exist in the seeded `response_code` table and the ISO spec assigns them distinct meanings).

Run: `./gradlew test --tests CheckCardTest --tests VerifySecurityTest --tests CheckLimitsTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/domain/txn/{CheckCard,VerifySecurity,CheckLimits}.java issuer-jpos/src/test/java/io/mcn/issuer/domain/txn/{CheckCardTest,VerifySecurityTest,CheckLimitsTest}.java
git commit -m "feat(iss): CheckCard, VerifySecurity (no-op until E5), CheckLimits participants (MCN-302a)"
```

---

### Task 4: `Authorize` (decline-only per the Ruling), `LogAndOutbox`, `Respond`

**Files:** `domain/txn/{Authorize,LogAndOutbox,Respond}.java`, tests

- [ ] **Step 1: Write the failing tests**

```java
package io.mcn.issuer.domain.txn;

import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;

class AuthorizeTest {

    @Test
    void noPriorDeclineFallsThroughToPendingLedgerNotApproval() {
        Context ctx = new Context(); // nothing set RESPONSE_CODE yet - would otherwise approve

        int result = new Authorize().prepare(1L, ctx);

        assertThat(result & PREPARED).isNotZero(); // not aborted - flows to LogAndOutbox/Respond
        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("96");
        assertThat(ctx.get(TxnContextKeys.DECLINE_REASON)).isEqualTo("ledger posting not implemented until MCN-302b");
    }

    @Test
    void priorDeclineIsPreserved() {
        Context ctx = new Context();
        ctx.put(TxnContextKeys.RESPONSE_CODE, "61");
        ctx.put(TxnContextKeys.DECLINE_REASON, "limit exceeded");

        new Authorize().prepare(1L, ctx);

        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61"); // unchanged
    }
}
```

```java
package io.mcn.issuer.domain.txn;

import io.mcn.issuer.adapter.persistence.TranLogRepository;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.mockito.Mockito;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.verify;

class LogAndOutboxTest {

    @Test
    void everyOutcomeWritesATranLogRow() throws Exception {
        var repo = Mockito.mock(TranLogRepository.class);
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, new ISOMsg("0200"));
        ctx.put(TxnContextKeys.RESPONSE_CODE, "14");
        ctx.put(TxnContextKeys.ACQUIRER_ID, "970499");

        new LogAndOutbox(repo).commit(1L, ctx);

        var captor = ArgumentCaptor.forClass(io.mcn.issuer.adapter.persistence.TranLogRow.class);
        verify(repo).insert(captor.capture());
        assertThat(captor.getValue().responseCode()).isEqualTo("14");
    }
}
```

```java
package io.mcn.issuer.domain.txn;

import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class RespondTest {

    @Test
    void buildsResponseWithRc14ForUnknownCard() throws Exception {
        ISOMsg request = new ISOMsg("0200");
        request.set(11, "000001");
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, request);
        ctx.put(TxnContextKeys.RESPONSE_CODE, "14");

        ISOMsg response = new Respond().buildResponse(ctx);

        assertThat(response.getMTI()).isEqualTo("0210");
        assertThat(response.getString(39)).isEqualTo("14");
        assertThat(response.getString(11)).isEqualTo("000001"); // DE 11 echoed
    }

    @Test
    void duplicateReplaysStoredResponseVerbatim() throws Exception {
        ISOMsg request = new ISOMsg("0200");
        ISOMsg stored = new ISOMsg("0210");
        stored.set(39, "00");
        stored.set(38, "123456");
        Context ctx = new Context();
        ctx.put(TxnContextKeys.REQUEST, request);
        ctx.put(TxnContextKeys.IS_DUPLICATE, true);
        ctx.put(TxnContextKeys.STORED_RESPONSE, stored);

        ISOMsg response = new Respond().buildResponse(ctx);

        assertThat(response.getString(39)).isEqualTo("00");
        assertThat(response.getString(38)).isEqualTo("123456");
    }
}
```

Run: `./gradlew test --tests AuthorizeTest --tests LogAndOutboxTest --tests RespondTest` → fail.

- [ ] **Step 2: Implement.** `Authorize.prepare` only sets RC 96 + the placeholder reason when `RESPONSE_CODE` is still unset (i.e., nothing upstream declined); it never overwrites an existing decline. `LogAndOutbox.commit` (not `prepare` — this only runs once the transaction is really finishing, matching jPOS's two-phase participant contract) builds a `TranLogRow` from the `Context` and calls `TranLogRepository.insert`. `Respond.buildResponse` builds the 0210/0110 response `ISOMsg` (repeat x21 responses keep the request's original MTI class per `docs/03` §4's function-digit rule — function digit 1 for a response), echoing every `E` (echo) field per the message catalogue table, setting DE 39 from `RESPONSE_CODE`, and — for a duplicate — replaying `STORED_RESPONSE`'s DE 38/39/4/54 verbatim per `docs/03` §7.5 rather than rebuilding them.

Run: `./gradlew test --tests AuthorizeTest --tests LogAndOutboxTest --tests RespondTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/domain/txn/{Authorize,LogAndOutbox,Respond}.java issuer-jpos/src/test/java/io/mcn/issuer/domain/txn/{AuthorizeTest,LogAndOutboxTest,RespondTest}.java
git commit -m "feat(iss): Authorize (decline-only stub), LogAndOutbox, Respond participants (MCN-302a)"
```

---

### Task 5: Wire the chain — `AuthorizationListener`, `TransactionManager` deploy descriptor, end-to-end socket test

**Files:** `adapter/iso/AuthorizationListener.java`, test; `src/dist/deploy/40_txn_manager.xml`; `PurchaseDeclineIntegrationTest.java`

- [ ] **Step 1: Write the failing integration test** — real socket, real `ISOServer`, real `TransactionManager`, real Testcontainers Postgres, extending MCN-201's `NetworkManagementIntegrationTest` harness pattern (sign on first, since RC 91 applies to an unsigned link):

```java
package io.mcn.issuer;

import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import static org.assertj.core.api.Assertions.assertThat;

@Testcontainers
class PurchaseDeclineIntegrationTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine")
            .withDatabaseName("issuer").withUsername("issuer").withPassword("test");

    @Test
    void purchaseOnUnsignedLinkIsDeclinedRc91() throws Exception {
        // real socket to a running ISOServer/AuthorizationListener/TransactionManager, no prior sign-on
        // send a 0200, assert 0210 DE39 == "91"
    }

    @Test
    void purchaseWithUnknownCardIsDeclinedRc14() throws Exception {
        // sign on first, then send 0200 with a PAN not in the seeded fixture set, assert RC 14
    }

    @Test
    void purchaseWithBlockedCardIsDeclinedRc62() throws Exception {
        // sign on, send 0200 using the fixture's tok_blocked PAN (9704360000003310), assert RC 62
    }

    @Test
    void duplicateRequestReplaysStoredResponse() throws Exception {
        // send the same 0200 (same STAN + DE7) twice; both responses must be byte-identical DE 39/38/4
    }

    @Test
    void wouldBeApprovalRespondsRc96WithLedgerNotImplementedReason() throws Exception {
        // sign on, send 0200 using tok_normal's PAN with a small amount within all limits -
        // per this story's Ruling, expect RC 96, not RC 00 (302b implements real approval)
    }
}
```

Check: fill in the real socket-framing code by copying `NetworkManagementIntegrationTest`'s exact connect/frame/send/receive helper methods rather than reinventing them — this is the established pattern for every ISS integration test in the repo.

Run: `./gradlew test --tests PurchaseDeclineIntegrationTest` → fails (`AuthorizationListener` doesn't exist).

- [ ] **Step 2: Implement `AuthorizationListener`** (an `ISORequestListener` that filters to MTI class `01`/`02`, checks `acquirer_link.status == 'SIGNED_ON'` before anything else and immediately responds RC 91 if not — per `docs/03` §7.1's "the issuer answers any request on a signed-off link with RC 91" rule — then hands off to a `TransactionManager` instance via `.queue(context)`), and `40_txn_manager.xml` registering the 8 participants in the exact order from the Goal section.

Run: `./gradlew test --tests PurchaseDeclineIntegrationTest -v` → PASS.

- [ ] **Step 3:** Wire `AuthorizationListener` into `src/dist/deploy/30_iso_server.xml` alongside `NetworkManagementListener` (jPOS's `ISOServer` supports multiple `<request-listener>` entries or a dispatching listener — check the real `ISOServer` config schema via the same sources-jar approach MCN-201 used before assuming the XML shape).

- [ ] **Step 4: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/iso/AuthorizationListener.java issuer-jpos/src/test/java/io/mcn/issuer/PurchaseDeclineIntegrationTest.java issuer-jpos/src/dist/deploy/40_txn_manager.xml issuer-jpos/src/dist/deploy/30_iso_server.xml
git commit -m "feat(iss): wire authorization TransactionManager chain into the ISO server (MCN-302a)"
```

---

### Task 6: Full verification and PR

- [ ] `./gradlew spotlessCheck build` clean; `ArchitectureTest`'s hexagonal ArchUnit rule still passes (domain participants have no framework/I/O imports beyond jPOS's own `TransactionParticipant` contract, which is this service's declared framework boundary, same as `ISORequestListener` for MCN-201/MCN-201's decision).
- [ ] AC → test table:

| AC (from MCN-302, this story's portion) | Test(s) |
| --- | --- |
| Participant chain order | `40_txn_manager.xml` + `PurchaseDeclineIntegrationTest` (proves the full chain runs end to end) |
| RC 14/62/54/61/13/12/91/30 | `CheckCardTest`, `CheckLimitsTest`, `ParseAndValidateTest`, `PurchaseDeclineIntegrationTest` |
| Duplicate replay | `DeduplicateTest`, `RespondTest`, `PurchaseDeclineIntegrationTest` |
| Every attempt logged | `LogAndOutboxTest` |

- [ ] Push, open PR `feat(iss): authorization chain - validate and decline paths (MCN-302a)`, explicitly stating in the PR body that RC 00/51 (real approval + ledger) are 302b's scope per the Ruling, so no reviewer mistakes RC 96 on a would-be-approved purchase for a bug.

## Self-review

- [ ] `Authorize`'s RC 96 fallback never overwrites a real decline set by an earlier participant — covered by `AuthorizeTest.priorDeclineIsPreserved`.
- [ ] Every path (decline or RC-96-placeholder) still calls `LogAndOutbox` — no `tran_log` row is ever skipped.
- [ ] `VerifySecurity`'s no-op is a real participant in the wired chain, not just a unit-tested class sitting unused — confirmed by `40_txn_manager.xml` listing it and `PurchaseDeclineIntegrationTest` exercising the full chain.
- [ ] No money-movement/ledger code was added in this story — grep the diff for any `journal_entry`/`ledger_posting`/`available_balance` write; there should be none (that's 302b).
