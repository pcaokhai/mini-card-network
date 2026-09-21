# Test Strategy and Scenarios

Version 1.0 · 2026-09-21 · Owner: Tech Lead

## 1. Principles

- Tests are written first (test-driven-development skill). A behavior without a test is not done.
- Test names reference AC ids: `should_decline_with_rc51_when_insufficient_funds__MCN_302_AC2`.
- The ledger invariant (Σ debits = Σ credits; balances = opening + postings) is asserted in every integration and chaos test that moves money.
- Deterministic tests: injected clocks, seeded random, no sleeps (await conditions with timeouts).

## 2. Test pyramid and tools

| Level | Scope | Tools | Owner lane | Runs |
| --- | --- | --- | --- | --- |
| Unit | Domain, application services, codec, reducers, components | JUnit 5 + AssertJ; Go `testing` + testify; Vitest + Testing Library | each | every PR |
| Property / fuzz | Codec unpack; reversal/duplicate interleavings; recon | jqwik (Java); Go fuzzing | ISS, GW, SET | every PR (short), nightly (long) |
| Integration | Repositories, participants chain, MUX + fake issuer, Kafka consumers | Testcontainers (Postgres, Kafka) | each | every PR |
| Contract — ISO | Golden vectors on both codecs; MAC vectors | vector runner in both repos | GW, ISS | every PR (`make contracts`) |
| Contract — REST/WS | Provider responses validated against OpenAPI; consumers use MSW from the same spec; breaking-change check | kin-openapi validator (Go), openapi4j/atlassian validator (Java), oasdiff, Spectral | GW, ISS, SET, WEB | every PR |
| Component UI | Screen states in both modes | Storybook + interaction tests, visual snapshots | WEB | every PR |
| End-to-end | Six critical journeys on the full stack | Playwright | WEB | nightly, release |
| Chaos | Ledger invariant under failures | `make chaos` (gateway chaos API + Toxiproxy) | PLAT | nightly (10k), PR label `chaos` (500) |
| Performance | 200 TPS for 10 min | Go load generator or k6 | PLAT | before each release from R3 |
| Security | PAN/PIN/track leak scan, secrets, dependency audit | `make pci-scan`, gitleaks, OWASP dependency-check, govulncheck, pnpm audit | PLAT | every PR |

## 3. Quality gates (CI blocks merge)

| Gate | Threshold |
| --- | --- |
| Unit + integration tests | 100% pass, no skipped tests without issue link |
| Line coverage (domain + application packages) | ≥ 85% Java/Go, ≥ 80% TS features; overall ≥ 75% |
| Mutation score on `domain` (PIT for Java, go-mutesting nightly) | ≥ 70% (reported from Sprint 4, gate from Sprint 6) |
| Contracts | vectors pass on both codecs; no breaking OpenAPI change without label + ADR |
| Static analysis | Spotless, Error Prone, ArchUnit; golangci-lint; eslint + tsc strict — zero errors |
| Security | zero `pci-scan` and gitleaks findings; no critical CVE |
| E2E (release branches) | six journeys green |
| Performance (release from R3) | p99 end-to-end < 300 ms at 200 TPS |

## 4. Test data

- Cards, terminals and keys come only from `contracts/fixtures/` (BIN 970436, acquirer 970499). Never real PANs.
- Every test that needs a card uses helpers (`TestCards.normal()`, `fixtures.cards.low`) — no inline PANs.
- Clock: `2026-09-21T07:32:00Z` default in tests; business date `2026-09-21`.

## 5. Critical E2E journeys (MCN-905)

1. Approved purchase from POS → visible in Overview feed → Journey shows 5 steps → card balance reduced.
2. Insufficient funds → RC 51 shown in POS, no ledger change.
3. Timeout with "drop response" chaos → POS shows reversal outcome → SAF delivers 0420 → card balance unchanged.
4. Block card in Cards screen → purchase declined RC 62 → audit line visible.
5. Rotate ZPK during 10 TPS background load → no failed transactions.
6. Cutover → reconcile with a seeded break → resolve → clearing file generated.

## 6. Acceptance test scenarios

### TS-01 Approved purchase (MCN-303, MCN-305, MCN-302)
**Objective:** A purchase with sufficient funds is approved, posted once and shown correctly.
**Starting conditions:** Stack up; link SIGNED_ON; card `tok_normal` balance 5,000,000; flags S3a on.
**Role:** Viewer at the POS.
**Steps:**
1. Select card "•••• 4417", chip + PIN, amount 250,000, press Pay → processing state appears.
2. Wait for result → "Payment approved", auth code shown, balance 4,750,000.
3. Open Journey → 5 steps, all OK; message panel shows 0200 and 0210 with masked PAN.
4. Open Cards → ledger has a PURCHASE entry with debit customer / credit settlement suspense 250,000.
**Expected:** exactly one tran_log row per host; journal balanced; WS `transaction.updated` received; trace visible in Tempo by RRN.

### TS-02 Idempotent retry (MCN-303-AC1)
**Objective:** Repeating a POST with the same Idempotency-Key does not charge twice.
**Starting conditions:** as TS-01.
**Steps:** 1. POST purchase with key K → 201 APPROVED. 2. POST again with key K, same body → 201 identical body. 3. POST with key K and a different amount → 422 `idempotency-key-mismatch`.
**Expected:** balance reduced once; one 0200 sent (issuer log).

### TS-03 Insufficient funds (MCN-302-AC2)
**Starting conditions:** card `tok_low` balance 80,000.
**Steps:** Pay 350,000 → declined.
**Expected:** status DECLINED, RC 51, Easy label "Not enough money in the account"; no journal entry; velocity counter unchanged.

### TS-04 Timeout and reversal (MCN-401, MCN-402)
**Objective:** Unknown outcome ends with money returned.
**Starting conditions:** chaos `DROP_RESPONSE` on; card `tok_normal` balance 5,000,000.
**Steps:**
1. Pay 600,000 → POS shows waiting, then "Transaction cancelled automatically" after 30 s.
2. Check SAF → one 0420 PENDING then ACKED.
3. Check issuer → original REVERSED, reversing journal present.
**Expected:** final balance 5,000,000; transaction status REVERSED; `mcn_reversal_total` +1; journey shows timeout, SAF and refund steps.

### TS-05 Reversal before original (MCN-402-AC2)
**Starting conditions:** fake issuer ordering: deliver 0420 before 0200 (Toxiproxy latency on first message only).
**Steps:** Send purchase; force timeout; reversal arrives first; original arrives later.
**Expected:** issuer records `REVERSAL_WITHOUT_ORIGINAL`, answers 0430; late original declined RC 94, not posted; ledger unchanged.

### TS-06 Duplicate request (MCN-402-AC3)
**Starting conditions:** chaos `DUPLICATE_REQUEST` on.
**Steps:** Pay 120,000.
**Expected:** issuer receives two identical 0200; second is replayed with same DE 38/39; one journal entry; `mcn_duplicate_total` +1.

### TS-07 SAF survives crash (MCN-401-AC4)
**Starting conditions:** chaos `CONNECTION_CUT` on; 50 purchases in flight.
**Steps:** 1. `kill -9` gateway. 2. Turn chaos off. 3. Restart gateway.
**Expected:** every timed-out transaction reaches REVERSED; SAF depth returns to 0; no DEAD items; ledger invariant holds.

### TS-08 Late response ignored (MCN-403)
**Starting conditions:** chaos `LATE_RESPONSE` (0210 delayed 35 s).
**Steps:** Pay 90,000; wait 40 s.
**Expected:** transaction REVERSED; `late_response_code = 00` stored; state does not change back to APPROVED; journey shows a WARN "late answer ignored" step.

### TS-09 Link recovery (MCN-202-AC4)
**Steps:** Disable the Toxiproxy proxy for 10 s, re-enable.
**Expected:** link DOWN within 3 missed echoes (accelerated echo interval in test config), network events written, reconnect and SIGNED_ON within 5 s of re-enable; header pill updates live.

### TS-10 Block card (MCN-308, MCN-309)
**Steps:** Block `crd_normal0001` (confirm) → pay 50,000 → unblock → pay 50,000.
**Expected:** first payment RC 62, second approved; two audit records with actor and before/after; lock overlay shown then removed.

### TS-11 Wrong PIN three times (MCN-503)
**Steps:** Pay with wrong PIN three times, then with right PIN.
**Expected:** RC 55, 55, 75; card status PIN_BLOCKED; fourth attempt RC 75; PIN block never appears in logs (`pci-scan`).

### TS-12 ZPK rotation under load (MCN-504)
**Steps:** Run 50 TPS; start rotation; wait for COMPLETED.
**Expected:** four steps DONE; new KCV on both sides; zero declined/failed transactions caused by keys during the grace window.

### TS-13 Pre-auth and completion (MCN-601, MCN-603)
**Steps:** Pre-auth 1,500,000 → available −1,500,000, ledger unchanged → complete 1,350,000.
**Expected:** journal for 1,350,000; hold COMPLETED; available restored by 150,000; completion advice acknowledged 0230.

### TS-14 Reconciliation with breaks (MCN-702, MCN-704, MCN-705)
**Starting conditions:** seeded day with one missing-at-issuer, one status mismatch, one amount mismatch.
**Steps:** cutover → 0500 (0510 RC 95) → reconcile → try clearing file → resolve three breaks → clearing file.
**Expected:** first file attempt 409 `open-breaks` shown inline; after resolution totals match; file deterministic (same SHA-256 on rerun); net position shown.

### TS-15 Stand-in (MCN-802)
**Steps:** Issuer down → pay 320,000 and 800,000 → issuer up.
**Expected:** circuit OPEN after 3 echo failures; 320,000 approved by STIP, 800,000 declined RC 91; advice drained after recovery; issuer ledger contains the STIP transaction.

### TS-16 Easy and Expert modes (MCN-004 and every screen story)
**Steps:** For each screen, toggle mode.
**Expected:** Easy mode has no raw codes except inside detail panels; Expert mode shows MTI/DE/RC; no layout shift; state preserved.

### TS-17 Reduced motion and accessibility (MCN-004, MCN-305)
**Steps:** Enable reduced motion; navigate POS with keyboard only; run axe on all screens.
**Expected:** no transform animations; keypad fully operable; zero serious/critical axe violations.

## 7. Chaos suite definition (MCN-407)

| Scenario | Toxic / mode | Invariant checked |
| --- | --- | --- |
| SLOW_NETWORK | latency 3,000 ms ± 200 | all final, p99 < timeout |
| CONNECTION_CUT | reset_peer every 20 s | all final after drain, SAF 0 |
| DROP_RESPONSE | fake-issuer drops 10% of 0210 | reversed count = dropped count |
| DUPLICATE_REQUEST | gateway double-sends 10% | one posting per RRN |
| ISSUER_DOWN | issuer paused 60 s | STIP or RC 91 only; drain after |
| LATE_RESPONSE | 5% of 0210 delayed 35 s | no REVERSED → APPROVED transitions |

Common invariant for every scenario: `mcn_ledger_discrepancy == 0`, Σ issuer approved − Σ issuer reversed = Σ acquirer approved − Σ acquirer reversed.
