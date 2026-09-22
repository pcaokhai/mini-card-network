# MCN-302b Issuer authorization chain — approve & ledger — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/iss-302b-authorize-ledger`, branch `feat/MCN-302b-authorize-ledger`. Requires MCN-302a merged (participant chain, `TranLogRepository`, decline paths). Part 2 of 2 for story MCN-302.

**Goal:** A purchase that clears every decline check gets really approved (RC 00, DE 38 auth code), posts a balanced double-entry journal (customer debit, settlement-suspense credit) in the same DB transaction as the `tran_log`/velocity-counter updates, and the whole chain survives 200 concurrent purchases on one card without breaking `chk_available_floor` or the ledger invariant, at p99 < 100ms.

**Architecture:** `Authorize` (MCN-302a's placeholder) is replaced with a real implementation: it reads the card's `account_id`, takes a row lock on `account` (`SELECT ... FOR UPDATE`, per the concurrency rule the schema's own `chk_available_floor` CHECK constraint backstops), debits `available_balance`/`ledger_balance`, inserts a `journal_entry` + two `ledger_posting` rows (customer debit, `SETTLEMENT_SUSPENSE` credit) — all inside the same connection/transaction `LogAndOutbox`'s `commit()` already participates in (jPOS's `TransactionManager` gives every participant in one transaction the same underlying DB connection via a `Context`-scoped resource, confirmed against the real API before assuming). `Authorize.prepare` computes the outcome (RC 00 + auth code, or RC 51 if insufficient funds) and stashes it in `Context`; the actual row lock + debit + journal insert happens in `Authorize.commit()` (jPOS two-phase: `prepare` decides, `commit` writes) — or, if the real `TransactionParticipant` contract makes that split awkward for a single participant, do the lock+debit+journal write in `prepare()` itself inside an explicit JDBC transaction and roll back on abort, whichever matches jPOS's real two-phase semantics once confirmed (see Task 1's check).

**Tech Stack:** Java 25, jPOS 3.0.1 `TransactionParticipant`, JDBC (`SELECT ... FOR UPDATE`), Testcontainers Postgres, JMH or a plain `ExecutorService`-based load test for the p99 measurement (no need for a heavier load-testing framework — YAGNI).

**Spec:** `docs/03-iso8583-interface-spec.md` §8 (RC 00 approved, RC 51 insufficient funds, DE 38 auth code), §9 (p99 < 100ms), `docs/05-data-model.md` §2 (`account`, `journal_entry`, `ledger_posting`, `gl_account`), `docs/assets/baseline-schema.sql` (the `chk_available_floor` CHECK and the deferred `trg_journal_balanced` trigger — both already exist from MCN-301's `V2__issuer_baseline.sql`, this story is the first to actually exercise them), `docs/06-user-stories.md` MCN-302 (items 3–5).

## Global Constraints

- Money stays `long` minor units throughout. Debit/credit amounts on `ledger_posting` are always positive; direction (`D`/`C`) carries the sign, matching the baseline schema's `chk` constraints.
- Every approval's journal must balance (Σ debits = Σ credits) — the baseline's `trg_journal_balanced` deferred constraint trigger already enforces this at COMMIT; this story must never try to work around it, only satisfy it.
- `chk_available_floor` (`available_balance >= -overdraft_limit`) is the last line of defense — this story's own `CheckLimits`/`Authorize` logic should decline with RC 51 well before the DB constraint would ever fire, but the constraint firing on a real bug is expected to surface as a hard failure, not something to catch and paper over.
- Auth code (DE 38) is a 6-character alphanumeric, generated per approval (e.g. a short random/sequential code — no external requirement on its exact algorithm beyond uniqueness-per-transaction; check `docs/03` §3 for any format constraint before inventing one).

## Ruling: where the row lock and journal write live in the participant chain

jPOS 3.0.1's `TransactionParticipant.prepare()`/`commit()`/`abort()` each receive the same `Context`, but confirming whether they also share one live DB `Connection` (vs. each participant getting/closing its own) is essential before deciding where the row lock belongs — a lock taken in `prepare()` on one connection and released when that connection closes before `commit()` runs would be a real correctness bug, not a style choice. **Read the real jPOS source/javadoc for `TransactionManager`'s connection-sharing behavior (or lack of it) before writing any code in this story** — if jPOS doesn't provide a shared-connection abstraction out of the box, `Authorize` opens its own explicit JDBC transaction spanning the row lock, debit, and journal insert entirely within a single participant method (most likely `prepare()`, since that's where the outcome must be decided before `Respond` can build DE 39/38), committing only if nothing failed, matching how MCN-301/302a's repositories already manage their own connections rather than relying on a framework-provided one. Document whichever real answer is found as the concrete implementation, replacing this Ruling's hedge once confirmed.

## File map

| Action | Path (under `issuer-jpos/`) |
| --- | --- |
| Modify | `src/main/java/io/mcn/issuer/domain/txn/Authorize.java` (or wherever MCN-302a's real merged location is — confirm the actual package, since MCN-302a's own report said participants live under `adapter/txn`, not the plan's originally-sketched `domain/txn`) |
| Create | `src/main/java/io/mcn/issuer/adapter/persistence/{LedgerRepository,AccountLockRepository}.java` + tests |
| Create | `src/main/java/io/mcn/issuer/adapter/txn/AuthCodeGenerator.java` + test |
| Create | `src/test/java/io/mcn/issuer/PurchaseApprovalIntegrationTest.java` |
| Create | `src/test/java/io/mcn/issuer/ConcurrentPurchaseLoadTest.java` |

---

### Task 0: Confirm jPOS's real connection-sharing behavior across one transaction's participants

- [ ] Read `org.jpos.transaction.TransactionManager`'s real source (via the sources jar, same technique every prior ISS story used) and any existing usage in this codebase (MCN-302a's participants) for how a DB connection is obtained/shared. Confirm: does `Context` carry a connection any participant can retrieve, or does each participant open its own? Write the answer as a one-paragraph note at the top of `Authorize.java`'s class javadoc so the next reader doesn't have to re-derive it.

---

### Task 1: `AccountLockRepository` — row-locked read + debit

**Files:** `adapter/persistence/AccountLockRepository.java`, test

**Interfaces:** `lockAndGet(Connection conn, long accountId) -> AccountRow` (`SELECT ... FOR UPDATE`, must be called within an existing transaction the caller manages — this repository never begins/commits its own transaction, unlike MCN-301's simpler repositories, because the lock's whole point is to be held across the debit + journal write); `debit(Connection conn, long accountId, long amount, long expectedVersion) -> boolean` (optimistic-version-checked `UPDATE ... WHERE id = ? AND version = ?`, returning whether it affected a row — belt-and-suspenders alongside the row lock, matching the baseline's `account.version` column).

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.persistence;

import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.sql.Connection;

import static org.assertj.core.api.Assertions.assertThat;

@Testcontainers
class AccountLockRepositoryTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine")
            .withDatabaseName("issuer").withUsername("issuer").withPassword("test");

    @Test
    void locksReadsAndDebitsWithinOneTransaction() throws Exception {
        var ds = TestDataSources.migrated(postgres);
        var accounts = new AccountRepository(ds);
        long accountId = accounts.insert("ACC-TEST-1", "704", 100_000L);
        var repo = new AccountLockRepository(ds);

        try (Connection conn = ds.getConnection()) {
            conn.setAutoCommit(false);
            var account = repo.lockAndGet(conn, accountId);
            assertThat(account.availableBalance()).isEqualTo(100_000L);

            boolean debited = repo.debit(conn, accountId, 10_000L, account.version());
            assertThat(debited).isTrue();
            conn.commit();
        }

        try (Connection conn = ds.getConnection()) {
            var after = repo.lockAndGet(conn, accountId);
            assertThat(after.availableBalance()).isEqualTo(90_000L);
            assertThat(after.version()).isEqualTo(1L);
        }
    }

    @Test
    void debitFailsWhenVersionIsStale() throws Exception {
        var ds = TestDataSources.migrated(postgres);
        var accounts = new AccountRepository(ds);
        long accountId = accounts.insert("ACC-TEST-2", "704", 50_000L);
        var repo = new AccountLockRepository(ds);

        try (Connection conn = ds.getConnection()) {
            boolean debited = repo.debit(conn, accountId, 1_000L, 999L); // wrong version
            assertThat(debited).isFalse();
        }
    }
}
```

Run: `./gradlew test --tests AccountLockRepositoryTest` → fails.

- [ ] **Step 2: Implement.** `AccountRow` record with `id, availableBalance, ledgerBalance, overdraftLimit, version`. `lockAndGet` issues `SELECT id, available_balance, ledger_balance, overdraft_limit, version FROM account WHERE id = ? FOR UPDATE`. `debit` issues `UPDATE account SET available_balance = available_balance - ?, ledger_balance = ledger_balance - ?, version = version + 1 WHERE id = ? AND version = ?` and returns `rowsAffected == 1`.

Run: `./gradlew test --tests AccountLockRepositoryTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/persistence/AccountLockRepository.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/persistence/AccountLockRepositoryTest.java
git commit -m "feat(iss): row-locked account read and optimistic-versioned debit (MCN-302b)"
```

---

### Task 2: `LedgerRepository` — journal entry + postings

**Files:** `adapter/persistence/LedgerRepository.java`, test

**Interfaces:** `postPurchase(Connection conn, long tranId, LocalDate businessDate, long accountId, long amount, String currency) -> long journalId` — inserts one `journal_entry` (`entry_type = 'PURCHASE'`) and two `ledger_posting` rows (customer `account_id` DEBIT `amount`, `SETTLEMENT_SUSPENSE` gl_code CREDIT `amount`), within the caller's transaction (same `Connection` parameter pattern as `AccountLockRepository`, for the same reason — the deferred balance trigger only fires at COMMIT, so both postings and the debit must share one transaction).

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.persistence;

import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.sql.Connection;
import java.time.LocalDate;

import static org.assertj.core.api.Assertions.assertThat;

@Testcontainers
class LedgerRepositoryTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine")
            .withDatabaseName("issuer").withUsername("issuer").withPassword("test");

    @Test
    void postsABalancedJournalForAPurchase() throws Exception {
        var ds = TestDataSources.migrated(postgres);
        var accounts = new AccountRepository(ds);
        long accountId = accounts.insert("ACC-LEDGER-1", "704", 100_000L);
        var tranLogRepo = new TranLogRepository(ds);
        long tranId = tranLogRepo.insert(sampleRow());
        var repo = new LedgerRepository(ds);

        try (Connection conn = ds.getConnection()) {
            conn.setAutoCommit(false);
            long journalId = repo.postPurchase(conn, tranId, LocalDate.now(), accountId, 10_000L, "704");
            conn.commit(); // trg_journal_balanced fires here - must not throw
            assertThat(journalId).isPositive();
        }
    }

    private TranLogRow sampleRow() { /* build a minimal valid row, same shape as MCN-302a's tests */ return null; }
}
```

Check: reuse MCN-302a's real `TranLogRow` constructor shape exactly (read the real merged file, don't guess the field order).

Run: `./gradlew test --tests LedgerRepositoryTest` → fails.

- [ ] **Step 2: Implement.** Insert into `journal_entry (tran_id, tran_business_date, entry_type)`, then two rows into `ledger_posting (journal_id, account_id, gl_code, direction, amount, currency)` — one with `account_id = <customer>, gl_code = NULL, direction = 'D'`, one with `account_id = NULL, gl_code = 'SETTLEMENT_SUSPENSE', direction = 'C'` (matching `ledger_posting`'s `chk_posting_target CHECK ((account_id IS NULL) <> (gl_code IS NULL))` from the baseline schema — exactly one of the two must be set per row).

Run: `./gradlew test --tests LedgerRepositoryTest -v` → PASS (a bug here — e.g. an unbalanced amount, or both `account_id`/`gl_code` set on one row — surfaces as a real constraint violation from Postgres at `conn.commit()`, which is the intended safety net, not something to catch and hide).

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/persistence/LedgerRepository.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/persistence/LedgerRepositoryTest.java
git commit -m "feat(iss): LedgerRepository posts a balanced double-entry journal per purchase (MCN-302b)"
```

---

### Task 3: `AuthCodeGenerator`

**Files:** `adapter/txn/AuthCodeGenerator.java`, test

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.txn;

import org.junit.jupiter.api.Test;
import static org.assertj.core.api.Assertions.assertThat;

class AuthCodeGeneratorTest {
    @Test
    void generatesSixAlphanumericCharacters() {
        String code = new AuthCodeGenerator().generate();
        assertThat(code).hasSize(6).matches("[A-Z0-9]{6}");
    }

    @Test
    void ratelyProducesDistinctCodes() {
        var gen = new AuthCodeGenerator();
        assertThat(gen.generate()).isNotEqualTo(gen.generate());
    }
}
```

Run: `./gradlew test --tests AuthCodeGeneratorTest` → fails.

- [ ] **Step 2: Implement** — a `SecureRandom`-backed 6-char base-36 uppercase generator; uniqueness is probabilistic (36^6 ≈ 2.2 billion combinations), acceptable for a teaching lab's transaction volume (YAGNI on a DB-backed sequence/dedupe check unless a real collision is ever observed).

Run: `./gradlew test --tests AuthCodeGeneratorTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/txn/AuthCodeGenerator.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/txn/AuthCodeGeneratorTest.java
git commit -m "feat(iss): 6-character auth code generator (MCN-302b)"
```

---

### Task 4: Replace `Authorize`'s placeholder with real approval + ledger posting

**Files:** `Authorize.java` (real merged location from MCN-302a), its existing test file

- [ ] **Step 1: Update the failing tests** — MCN-302a's `AuthorizeTest.noPriorDeclineFallsThroughToPendingLedgerNotApproval` must be rewritten (its whole premise — "ledger posting not implemented" — is what this story replaces):

```java
package io.mcn.issuer.adapter.txn; // or wherever MCN-302a's real package is

import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.when;

class AuthorizeTest {

    @Test
    void sufficientFundsApprovesAndPostsLedger() throws Exception {
        var lockRepo = Mockito.mock(AccountLockRepository.class);
        var ledgerRepo = Mockito.mock(LedgerRepository.class);
        when(lockRepo.lockAndGet(any(), anyLong())).thenReturn(
                new io.mcn.issuer.adapter.persistence.AccountRow(1L, 100_000L, 100_000L, 0L, 0L));
        when(lockRepo.debit(any(), anyLong(), anyLong(), anyLong())).thenReturn(true);

        Context ctx = new Context();
        ctx.put(TxnContextKeys.ACCOUNT_ID, 1L);
        ctx.put(TxnContextKeys.AMOUNT, 10_000L);
        ctx.put(TxnContextKeys.RESPONSE_CODE, null);

        new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator()).prepare(1L, ctx);

        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
        assertThat(((String) ctx.get(TxnContextKeys.AUTH_CODE))).hasSize(6);
    }

    @Test
    void insufficientFundsDeclinesRc51WithoutPostingLedger() throws Exception {
        var lockRepo = Mockito.mock(AccountLockRepository.class);
        var ledgerRepo = Mockito.mock(LedgerRepository.class);
        when(lockRepo.lockAndGet(any(), anyLong())).thenReturn(
                new io.mcn.issuer.adapter.persistence.AccountRow(1L, 5_000L, 5_000L, 0L, 0L));

        Context ctx = new Context();
        ctx.put(TxnContextKeys.ACCOUNT_ID, 1L);
        ctx.put(TxnContextKeys.AMOUNT, 10_000L);

        new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator()).prepare(1L, ctx);

        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("51");
        Mockito.verifyNoInteractions(ledgerRepo);
    }

    @Test
    void priorDeclineFromAnEarlierParticipantIsPreservedUnchanged() {
        var lockRepo = Mockito.mock(AccountLockRepository.class);
        var ledgerRepo = Mockito.mock(LedgerRepository.class);
        Context ctx = new Context();
        ctx.put(TxnContextKeys.RESPONSE_CODE, "61");

        new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator()).prepare(1L, ctx);

        assertThat(ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61");
        Mockito.verifyNoInteractions(lockRepo, ledgerRepo);
    }
}
```

Run: `./gradlew test --tests AuthorizeTest` → fails.

- [ ] **Step 2: Implement.** Per Task 0's finding on connection sharing: if `Context` doesn't carry a shared connection, `Authorize.prepare` opens its own `Connection` from a `DataSource` (constructor-injected), begins a transaction, calls `lockRepo.lockAndGet`, compares `AMOUNT` against `availableBalance` (RC 51 if insufficient — abort, roll back, no debit/journal), else `lockRepo.debit` + `ledgerRepo.postPurchase`, commits, stores `AUTH_CODE`/`RESPONSE_CODE=00` in `Context`. If an earlier participant already set `RESPONSE_CODE`, short-circuit immediately without touching the account (per `priorDeclineFromAnEarlierParticipantIsPreservedUnchanged`).

Check: this participant must still implement `AbortParticipant` (per MCN-302a's real finding) if it needs cleanup on abort — since it manages its own transaction and rolls back explicitly on any decline path within `prepare()` itself, an `abort()` override may not need to do anything extra, but confirm this against the same jPOS behavior MCN-302a already reverse-engineered rather than re-deriving it from scratch.

Run: `./gradlew test --tests AuthorizeTest -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/txn/Authorize.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/txn/AuthorizeTest.java
git commit -m "feat(iss): real approval (RC 00 + auth code) and ledger posting, RC 51 on insufficient funds (MCN-302b)"
```

---

### Task 5: `PurchaseApprovalIntegrationTest` — real socket, real approval

**Files:** `src/test/java/io/mcn/issuer/PurchaseApprovalIntegrationTest.java`

- [ ] **Step 1: Write the test** (extends MCN-302a's `PurchaseDeclineIntegrationTest` harness, real embedded Q2 against `src/dist/deploy`):

```java
@Test
void approvedPurchaseReturnsRc00WithAuthCodeAndPostsLedger() throws Exception {
    // sign on, seed an account with sufficient balance, send 0200 for tok_normal's card at a small
    // amount, assert 0210 DE39 == "00" and DE38 is a 6-char code, then query journal_entry/ledger_posting
    // directly to confirm one balanced entry was posted for this tran_id.
}

@Test
void purchaseExceedingBalanceIsDeclinedRc51() throws Exception {
    // use tok_low's seeded low-balance account with an amount exceeding it, assert RC 51,
    // and confirm no journal_entry row was created for this tran_id.
}
```

Run: `./gradlew test --tests PurchaseApprovalIntegrationTest` → fails until Task 4 lands, then PASS.

- [ ] **Step 2: Commit**

```bash
git add issuer-jpos/src/test/java/io/mcn/issuer/PurchaseApprovalIntegrationTest.java
git commit -m "test(iss): real socket approval and insufficient-funds integration tests (MCN-302b)"
```

---

### Task 6: `ConcurrentPurchaseLoadTest` — 200 concurrent purchases, ledger invariant, p99 < 100ms (AC4/AC5)

**Files:** `src/test/java/io/mcn/issuer/ConcurrentPurchaseLoadTest.java`

- [ ] **Step 1: Write the test**

```java
package io.mcn.issuer;

import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.util.concurrent.*;
import java.util.stream.IntStream;

import static org.assertj.core.api.Assertions.assertThat;

@Testcontainers
class ConcurrentPurchaseLoadTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine")
            .withDatabaseName("issuer").withUsername("issuer").withPassword("test");

    @Test
    void twoHundredConcurrentPurchasesOnOneCardNeverBreakTheLedgerInvariant() throws Exception {
        // Seed one account with a balance that allows roughly half of the 200 concurrent purchase
        // attempts (each a fixed small amount) to succeed before running dry - deliberately forces
        // both APPROVED and RC-51-DECLINED outcomes to race against each other on the same account.
        var ds = TestDataSources.migrated(postgres);
        var accounts = new AccountRepository(ds);
        long amount = 1000L;
        long startingBalance = amount * 100; // exactly 100 of the 200 attempts can succeed
        long accountId = accounts.insert("ACC-LOAD-1", "704", startingBalance);

        var lockRepo = new AccountLockRepository(ds);
        var ledgerRepo = new LedgerRepository(ds);
        var tranLogRepo = new TranLogRepository(ds);
        var authorize = new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator());

        ExecutorService pool = Executors.newFixedThreadPool(50);
        var latencies = new CopyOnWriteArrayList<Long>();
        var approvedCount = new java.util.concurrent.atomic.AtomicInteger();

        var futures = IntStream.range(0, 200).mapToObj(i -> pool.submit(() -> {
            long start = System.nanoTime();
            var ctx = new org.jpos.transaction.Context();
            ctx.put(TxnContextKeys.ACCOUNT_ID, accountId);
            ctx.put(TxnContextKeys.AMOUNT, amount);
            authorize.prepare(i, ctx);
            latencies.add((System.nanoTime() - start) / 1_000_000);
            if ("00".equals(ctx.get(TxnContextKeys.RESPONSE_CODE))) approvedCount.incrementAndGet();
        })).toList();
        for (var f : futures) f.get(30, TimeUnit.SECONDS);
        pool.shutdown();

        // Invariant 1: exactly 100 approvals (never more - the row lock must serialize every debit)
        assertThat(approvedCount.get()).isEqualTo(100);

        // Invariant 2: final available_balance is exactly startingBalance - (100 * amount) = 0,
        // never negative, never double-counted
        try (var conn = ds.getConnection()) {
            var account = lockRepo.lockAndGet(conn, accountId);
            assertThat(account.availableBalance()).isEqualTo(0L);
        }

        // Invariant 3: exactly 100 journal_entry rows exist for this account's purchases, each balanced
        // (query journal_entry/ledger_posting directly and sum debits vs credits per journal_id)

        // p99 latency assertion (AC5's "processing target", exercised here as a proxy for issuer
        // processing time under load - the real end-to-end p99 also includes network round-trip,
        // measured properly at the integration-checkpoint level, not unit-testable in isolation)
        var sorted = latencies.stream().sorted().toList();
        long p99 = sorted.get((int) (sorted.size() * 0.99));
        assertThat(p99).isLessThan(100L);
    }
}
```

Run: `./gradlew test --tests ConcurrentPurchaseLoadTest` → PASS once Task 4 is correct; a race in the row-lock/optimistic-version logic shows up here as `approvedCount` drifting from exactly 100, or the final balance landing below zero — both real, valuable failures, not flaky noise, if the locking is actually broken.

- [ ] **Step 2: Commit**

```bash
git add issuer-jpos/src/test/java/io/mcn/issuer/ConcurrentPurchaseLoadTest.java
git commit -m "test(iss): 200 concurrent purchases hold the ledger invariant and p99 target (MCN-302b AC4/AC5)"
```

---

### Task 7: Full verification and PR

- [ ] `./gradlew spotlessCheck build` clean, including `ArchitectureTest`.
- [ ] AC → test table:

| AC (MCN-302, this story's portion) | Test(s) |
| --- | --- |
| Real approval, RC 00 + DE 38 | `AuthorizeTest.sufficientFundsApprovesAndPostsLedger`, `PurchaseApprovalIntegrationTest` |
| RC 51 insufficient funds | `AuthorizeTest.insufficientFundsDeclinesRc51WithoutPostingLedger`, `PurchaseApprovalIntegrationTest` |
| Balanced journal posted in the same transaction | `LedgerRepositoryTest`, `AccountLockRepositoryTest` |
| 200 concurrent purchases hold the ledger invariant | `ConcurrentPurchaseLoadTest` |
| p99 < 100ms | `ConcurrentPurchaseLoadTest`'s latency assertion |

- [ ] Push, open PR `feat(iss): real approval, double-entry ledger, concurrency and perf proof (MCN-302b)`.

## Self-review

- [ ] The row lock (`SELECT ... FOR UPDATE`) and the debit + journal insert genuinely share one DB transaction/connection — verified by Task 0's real confirmation, not assumed.
- [ ] No approval ever posts an unbalanced journal — the baseline's `trg_journal_balanced` deferred trigger is the real backstop, exercised for real by every test that calls `conn.commit()`.
- [ ] `ConcurrentPurchaseLoadTest`'s invariant assertions (exact approval count, exact final balance) would actually fail if the locking were broken — not a tautological test that passes regardless of correctness.
- [ ] `Authorize`'s decline paths (RC 51, or an already-set `RESPONSE_CODE` from an earlier participant) never touch `AccountLockRepository`/`LedgerRepository` — verified by `Mockito.verifyNoInteractions` in the relevant tests.
