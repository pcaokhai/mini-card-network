# User Stories

Version 1.0 · 2026-09-21 · Format: 3 C's + INVEST. Design: https://claude.ai/artifact/LUUvgVcXYjqkHwdRui1Jmw

Each story lists: **Lane** (ISS, GW, WEB, SET, PLAT) · **Points** (Fibonacci) · **Slice** (paired BE+FE release unit, see `07-delivery-plan.md`) · **Depends on** · **Traces** (FR/NFR). Acceptance criteria IDs (`MCN-303-AC2`) must appear in the names of the tests that prove them.

Roles: *Learner* (engineer using the lab), *Operator* (runs the network), *Viewer* (non-technical), *Developer* (builds the system).

---

## E0 — Platform and contracts (Sprint 0)

### MCN-001 Monorepo scaffold and CI
Lane PLAT · 5 pts · Depends: — · Traces: NFR-09
As a Developer, I want one repository with consistent commands and CI, so that every lane builds and tests the same way.
1. `make up|down|test|lint|fmt|contracts|e2e` exist and delegate to each service.
2. GitHub Actions runs lint + unit tests per changed service and `make contracts` on every PR; path filters skip unaffected services.
3. Branch protection: PR required, CI green, 1 review, squash merge, Conventional Commit title check.
4. PR template (`.github/pull_request_template.md`) and CODEOWNERS per lane exist.
5. Tech stack versions pinned and recorded in `docs/02` §9.

### MCN-002 Local infrastructure
Lane PLAT · 5 pts · Depends: MCN-001 · Traces: NFR-08
As a Developer, I want the whole platform to start with one command, so that I can run end-to-end flows locally.
1. `make up` starts Postgres (3 DBs, 3 roles), Kafka (KRaft), Toxiproxy (proxy `issuer` 18000→8000), OTel Collector, Tempo, Prometheus, Grafana with healthchecks.
2. `make up` finishes in < 90 s on a warm machine; `make down -v` removes all state.
3. `.env.example` documents every variable; no secret is committed.
4. Seed command loads `contracts/fixtures/` into the issuer and acquirer databases.

### MCN-003 Contracts v0
Lane PLAT · 3 pts · Depends: MCN-001 · Traces: FR-18
As a Developer, I want versioned contracts with automated checks, so that lanes can build in parallel safely.
1. `openapi.yaml` passes Spectral lint with the project ruleset (naming, problem responses, Idempotency-Key on writes).
2. `oasdiff` breaking-change check fails CI for breaking changes without an `api-breaking` label and ADR link.
3. ISO vectors in `contracts/iso8583/vectors/` are validated against `packager-spec.yaml` by a script.
4. WS and Kafka schemas compile with a JSON Schema validator in CI.

### MCN-004 Web application shell
Lane WEB · 5 pts · Slice S0 · Depends: MCN-001 · Traces: FR-18
As a Viewer, I want a clean console with navigation and an Easy/Expert switch, so that I can find every screen and choose my level of detail.
1. Layout matches the design: sidebar with 9 entries in 3 groups, header with search box, link status pill and mode toggle.
2. Mode toggle persists per browser and switches `<Term>` labels app-wide without layout shift (MCN-004-AC2 story in Storybook).
3. Design tokens and motion tokens exist in `shared/`; `prefers-reduced-motion` reduces all motion to fades.
4. `vi` and `en` message files load; Vietnamese is default.
5. MSW mock mode works with generated handlers; Storybook runs with the design-system components.

### MCN-005 Service skeletons with observability
Lane ISS, GW, SET (parallel) · 5 pts · Depends: MCN-002 · Traces: NFR-08, NFR-09
As an Operator, I want every service to expose health, logs, metrics and traces from day one, so that problems are visible immediately.
1. Each service exposes `/health/live`, `/health/ready` and Prometheus metrics.
2. Logs are JSON with the fields in `docs/02` §7.6; the masker is installed and unit-tested with a PAN sample.
3. A request through the BFF to each service produces one trace in Tempo.
4. SIGTERM triggers graceful shutdown (stop accepting, drain ≤ 30 s, exit 0).

---

## E1 — Message codec and Message Lab (Sprint 1)

### MCN-101 ISO 8583 codec (Go)
Lane GW · 8 pts · Depends: MCN-003 · Traces: FR-18, ADR-003
As a Learner, I want a codec I built and understand, so that I know exactly how bytes become fields.
1. Packs and unpacks MTI, primary/secondary ASCII-hex bitmap, fixed n/an/ans, LLVAR, LLLVAR and binary-as-hex per `packager-spec.yaml` (generated field table).
2. All golden vectors round-trip byte-identical; invalid vectors return typed errors (`ErrInvalidLength`, `ErrUnknownField`, …) with the DE number.
3. Fuzz test on unpack runs 60 s in CI without panics.
4. Cross-check test packs every vector with `moov-io/iso8583` (test-only) and compares.

### MCN-102 jPOS packager and vector tests
Lane ISS · 3 pts · Depends: MCN-003 · Traces: FR-04
As a Developer, I want the issuer to parse exactly the same bytes as the gateway, so that both sides agree on the wire.
1. `iso87ascii.xml` is generated from `packager-spec.yaml` by a Gradle task.
2. All golden vectors pack/unpack byte-identical with the generated `GenericPackager`.
3. The issuer log formatter masks DE 2, 35, 45, 52, 55 and shortens 64/128.

### MCN-103 Lab API
Lane GW · 3 pts · Slice S1 · Depends: MCN-101 · Traces: FR-18
As a Learner, I want to decode, encode and load sample messages through an API, so that the web lab can explain any message.
1. `POST /v1/lab/messages/decode` returns MTI, bitmaps, ordered segments and fields with easy and technical names.
2. `POST /v1/lab/messages/encode` validates fields and returns the packed message and the same breakdown.
3. `GET /v1/lab/messages/samples` returns the four golden samples.
4. PAN is masked in every response; responses validate against the OpenAPI spec.

### MCN-104 Message Lab screen
Lane WEB · 5 pts · Slice S1 · Depends: MCN-003 (mocks) · Traces: FR-18
As a Learner, I want to see a message byte by byte with its bitmap, so that I understand how fields are found.
1. Raw message is shown as coloured segments (MTI, bitmap, each field); selecting a segment, a bitmap cell or a table row highlights the same element in all three.
2. 8×8 bitmap grid with per-row hex and binary; secondary bitmap tab appears when bit 1 is set.
3. Detail panel explains the selected element; MTI digits, DE 22, DE 55 tags and DE 90 parts are broken down.
4. Easy/Expert modes change names and show format column only in Expert.
5. Bitmap cells animate with a diagonal flip when the message changes (reduced motion: fade).

---

## E2 — Connectivity (Sprint 2)

### MCN-201 Issuer ISO server and network management
Lane ISS · 5 pts · Depends: MCN-102 · Traces: FR-02, FR-03
As an Operator, I want the issuer to accept connections and answer sign-on and echo, so that links can be established and monitored.
1. QServer on port 8000 with NACChannel; listener only enqueues to the TransactionManager.
2. 0800 with DE 70 = 001/002/301 answered with 0810 RC 00 and link state updated in `acquirer_link`.
3. Financial requests on a signed-off link are answered with RC 91.
4. Malformed message ⇒ 0x10/0x30 with RC 30 when MTI is readable; otherwise the connection is closed and logged.

### MCN-202 Gateway connection manager
Lane GW · 8 pts · Depends: MCN-101 · Traces: FR-02, FR-03, NFR-04
As an Operator, I want the gateway to keep the link healthy on its own, so that transactions flow without manual action.
1. On start: connect, sign on, start echo every 60 s; on shutdown: sign off, then close.
2. 3 consecutive echo failures ⇒ link DOWN, close, reconnect with backoff 1–30 s + full jitter, sign on again.
3. `link_state` and `mcn_link_up` reflect reality within 1 s of a change; a `network_event` row is written per change.
4. Pulling the Toxiproxy link and restoring it recovers within 5 s (integration test).

### MCN-203 MUX
Lane GW · 5 pts · Depends: MCN-202 · Traces: FR-04, FR-10
As a Developer, I want responses matched to requests reliably, so that concurrent transactions never get each other's answers.
1. Pending request registered by (DE 11, DE 7) before the socket write; responses in any order resolve the right waiter.
2. Timeout returns `ErrTimeout` after the configured duration; the pending entry is removed.
3. A response with no pending entry calls the late-response hook and increments `mcn_late_response_total`.
4. 1,000 concurrent requests with randomised response order pass under `-race`.

### MCN-204 Network API and events
Lane GW · 3 pts · Slice S2 · Depends: MCN-202 · Traces: FR-18
As an Operator, I want to see and control links from the console, so that I can operate without a terminal.
1. `GET /v1/network/links`, `POST …/echo|sign-on|sign-off`, `GET /v1/network/events` implemented per spec.
2. WS emits `link.status` and `network.event` on every change.
3. Manual echo returns latency and RC; on a down link returns `ok=false` without throwing.

### MCN-205 Network Operations screen v1
Lane WEB · 5 pts · Slice S2 · Depends: MCN-003 (mocks) · Traces: FR-18
As an Operator, I want a topology and link table, so that I can see at a glance whether the network is healthy.
1. Topology shows POS → Acquirer → Issuer with link colour by status; links animate in the request direction when up, red dashed when down.
2. Links table with status, latency, last echo and a "Check now" button (echo).
3. Event timeline updates live from WS; newest first.
4. Header link pill reflects the gateway→issuer link on every screen.

---

## E3 — Authorization and POS (Sprints 3–4)

### MCN-301 Issuer schema and seed
Lane ISS · 3 pts · Depends: MCN-201 · Traces: FR-05
As a Developer, I want the issuer schema and test data in place, so that authorization can be built on it.
1. First Flyway migration = issuer part of the baseline + deltas for issuer in `docs/05` §4.
2. Seed loads fixture cards/accounts with encrypted PAN, HMAC and masked PAN.
3. Repository integration tests run on Testcontainers.

### MCN-302 Issuer authorization chain
Lane ISS · 13 pts (split 302a validate/respond 5, 302b authorize/ledger 8) · Depends: MCN-301 · Traces: FR-04, FR-05, NFR-01, NFR-02
As a Cardholder, I want my purchase approved or declined correctly and instantly, so that I can pay or know why not.
1. Participants in order `ParseAndValidate → Deduplicate → CheckCard → VerifySecurity (no-op until E5) → CheckLimits → Authorize → LogAndOutbox → Respond`.
2. RC per ISO spec §8: 14 unknown card, 62 blocked, 54 expired, 61 limit, 51 insufficient funds, 13 invalid amount, 00 approved with DE 38.
3. Approval posts a balanced journal (customer debit, settlement suspense credit), updates balances and velocity counters, writes tran_log and outbox in one DB transaction.
4. 200 concurrent purchases on one card never break `chk_available_floor` and the ledger invariant holds (MCN-302-AC4 concurrency test).
5. p99 issuer processing < 100 ms at 200 TPS in the load spike (records assumption A2).

### MCN-303 Purchase flow in the gateway
Lane GW · 8 pts · Slice S3a · Depends: MCN-203, MCN-302 (contract only) · Traces: FR-01, FR-04, NFR-05
As a Cardholder at a POS, I want to pay with my card, so that I can buy goods.
1. `POST /v1/transactions/purchases` requires Idempotency-Key; replays return the stored response.
2. Builds 0200 per spec (STAN, RRN, DE 7/12/13/15), sends through MUX, maps 0210 to status and `responseLabel`.
3. State machine persists every transition in `tran_state_history`; illegal transitions are rejected by tests.
4. Link not signed on ⇒ transaction `DECLINED` RC 91 without sending.
5. Response validates against OpenAPI; WS `transaction.created/updated` emitted.

### MCN-304 Transactions query and journey API
Lane GW · 5 pts · Slice S3b · Depends: MCN-303 · Traces: FR-18
As a Learner, I want to fetch transactions and their step-by-step journey, so that the console can replay what happened.
1. List with filters and cursor pagination, newest first; detail by RRN.
2. Journey returns ordered steps with actor, offset, easy/technical texts, kind, and masked ISO messages from stored exchanges.
3. Money timeline rows are computed from issuer-reported balance deltas (via 0210 DE 54 for lab, or null when unknown).

### MCN-305 POS simulator
Lane WEB · 8 pts · Slice S3a · Depends: MCN-003 (mocks) · Traces: FR-01
As a Viewer, I want a realistic POS where I choose a card and pay, so that I can create transactions without technical knowledge.
1. Keypad (keyboard accessible), four test cards, entry modes, five one-click scenarios as in the design.
2. Pay shows a processing state (spinner and animated dots) until the API returns; the button is locked during processing.
3. Result panel shows outcome, plain explanation and steps; Expert mode adds MTI/STAN/RC line; decline icon shakes, approval pops.
4. PIN is encrypted client-side with the simulator TPK before sending; PIN digits are cleared from state after submit.
5. Network scenario displays the timeout-and-reversal outcome with `REVERSAL_PENDING`/`REVERSED` statuses.

### MCN-306 Overview dashboard and live feed
Lane WEB · 5 pts · Slice S3b · Depends: MCN-003 (mocks) · Traces: FR-18
As an Operator, I want KPIs and a live feed, so that I know how the network behaves right now.
1. Four KPI cards, 60-minute throughput bars, decline reasons, system health list, cutover countdown.
2. New transactions slide into the feed from WS with a status flash; above 5 events/s updates batch every 500 ms; max 30 rows.
3. Expert mode shows RRN column and RC codes.

### MCN-307 Transaction Journey screen (success path)
Lane WEB · 5 pts · Slice S3b · Depends: MCN-003 (mocks) · Traces: FR-18
As a Viewer, I want to replay a transaction step by step, so that I understand what happened between the banks.
1. Summary header, step timeline, step detail with message fields, money panel, as in the design.
2. Autoplay, step back/forward, restart; current step pulses; future steps are dimmed.
3. Opening a transaction from the feed, POS or search navigates to `/transactions/{rrn}`.

### MCN-308 Issuer Admin API
Lane ISS · 5 pts · Slice S3c · Depends: MCN-302 · Traces: FR-18, NFR-07
As an Operator, I want to manage cards and see their ledger, so that I can support cardholders.
1. Endpoints `/v1/cards…` per spec, including ETag/If-Match on limits and Idempotency-Key on writes.
2. Block/unblock and limit changes write `audit_log` with actor and before/after.
3. Ledger endpoint returns journal entries with postings; every entry balances.

### MCN-309 Cards and Accounts screen
Lane WEB · 5 pts · Slice S3c · Depends: MCN-003 (mocks) · Traces: FR-18
As an Operator, I want to see a card's balances, holds, limits and ledger, so that I can answer "where did my money go?".
1. Card list with status badges; detail with card visual, three-line balance, holds, limits sliders with usage bar, ledger table.
2. Block/unblock requires inline confirmation; a lock overlay animates on the card; an audit line appears.
3. Limit change sends If-Match; a 412 shows "Someone changed this card. Reload to continue."

---

## E4 — Resilience (Sprints 5–6)

### MCN-401 Timeout → reversal and SAF
Lane GW · 8 pts · Slice S4a · Depends: MCN-303 · Traces: FR-07, FR-08, NFR-02, NFR-03
As a Cardholder, I want never to lose money when the network fails, so that I can trust card payments.
1. On timeout: in one DB transaction set `TIMED_OUT → REVERSAL_PENDING` and insert a 0420 (DE 39 = 68, DE 90 per spec) into `saf_queue`; POS gets RC 68.
2. SAF worker claims with `SKIP LOCKED`, sends 0420 then 0421 repeats with backoff (2 s base, 60 s cap, jitter), marks ACKED on 0430 and moves the transaction to `REVERSED`.
3. After `max_attempts` the item becomes DEAD, `mcn_saf_dead_total` increments and a WARN network event is written.
4. `kill -9` during load then restart: every pending reversal is delivered (MCN-401-AC4 integration test).
5. `POST /v1/transactions/{rrn}/cancellations` produces a 0420 with DE 39 = 17 through the same path.

### MCN-402 Issuer reversal, advice and duplicate handling
Lane ISS · 8 pts · Slice S4a · Depends: MCN-302 · Traces: FR-07, FR-09
As an Issuer, I want reversals and duplicates handled safely, so that the ledger is always right.
1. 0420/0421 locate the original via DE 90; found ⇒ reversing journal, status REVERSED; answer 0430 after commit.
2. Not found ⇒ record `REVERSAL_WITHOUT_ORIGINAL`, answer 0430; a later original is declined with RC 94 and not posted.
3. Duplicate requests replay `stored_response` byte-for-byte; repeated reversals are idempotent.
4. Property-based test: any interleaving of original, duplicate, reversal and repeat leaves Σ ledger consistent.

### MCN-403 Late response handling
Lane GW · 3 pts · Depends: MCN-401 · Traces: FR-10
As an Operator, I want late answers recorded but ignored, so that a reversed transaction never flips back to approved.
1. A 0210 arriving for a transaction not in `SENT` stores `late_response_code/at`, keeps the state, emits metric and event.
2. Journey shows the late response as a WARN step.

### MCN-404 Chaos API
Lane GW · 3 pts · Slice S4b · Depends: MCN-401 · Traces: FR-18
As a Learner, I want to switch failures on and off and run synthetic load, so that I can watch the system protect money.
1. Six scenarios map to Toxiproxy toxics (latency 3000 ms; reset_peer; drop responses via fake-issuer mode; duplicate send; issuer down; delayed response > timeout).
2. `POST /v1/chaos/runs` runs N synthetic purchases on seed cards and ends with a money verification (opening − approved + reversed = closing, discrepancy 0).
3. WS `chaos.changed` and `chaos.run.progress` emitted.

### MCN-405 Chaos Lab screen
Lane WEB · 5 pts · Slice S4b · Depends: MCN-003 (mocks) · Traces: FR-18
As a Viewer, I want failure switches and a live proof that money is safe, so that I understand why payment systems are designed this way.
1. Six scenario cards with toggle; active cards highlighted; header shows count of active failures.
2. Money verification panel updates during a run and flashes the zero discrepancy on completion; a non-zero discrepancy is shown in red with the run id.
3. Reaction panel lists what the system does for each active failure (Easy vs Expert text).

### MCN-406 Journey for failure scenarios
Lane WEB · 3 pts · Slice S4a · Depends: MCN-307 · Traces: FR-18
As a Viewer, I want to replay timeouts, reversals, SAF retries and late responses, so that I can see how the network recovers.
1. Journey renders REVERSAL, WARN and BAD steps with the money panel showing debit then refund.
2. A 30-second timeout is visualised with an accelerated countdown ring during autoplay.

### MCN-407 Chaos test suite
Lane PLAT · 5 pts · Depends: MCN-401, MCN-402 · Traces: NFR-02
As a Developer, I want an automated proof of the ledger invariant under chaos, so that regressions are caught in CI.
1. `make chaos` runs 10,000 transactions per scenario (nightly) and 500 per scenario (PR, labelled `chaos`).
2. Fails if `mcn_ledger_discrepancy ≠ 0`, any SAF item is DEAD, or any transaction stays non-final after drain.
3. Produces a markdown report artifact with counts per status and RC.

---

## E5 — Security (Sprints 6–7)

### MCN-501 Security module adapters and key store
Lane ISS + GW (parallel) · 8 pts · Depends: MCN-302, MCN-303 · Traces: FR-11, NFR-06
As a Security officer, I want keys stored only as cryptograms with KCVs, so that no one can read a clear key.
1. Issuer: `SecurityModule` port implemented with JCESecurityModule; gateway: `hsm` simulator implementing PIN translate and MAC.
2. Key inventory endpoints return type, KCV, status, days remaining; no clear key material anywhere.
3. LMK test value from env; missing LMK fails startup.

### MCN-502 PIN translation and MAC (gateway)
Lane GW · 5 pts · Depends: MCN-501 · Traces: FR-11
As an Acquirer, I want PINs translated and messages signed, so that data is protected between banks.
1. TPK→ZPK translation for DE 52; MAC over the packed message per ISO spec §11 with ZAK in DE 64/128.
2. MAC vectors added to `contracts/iso8583/vectors/mac/`; both sides pass them.
3. Incoming 0210 with a bad MAC ⇒ transaction FAILED with reversal queued, `mcn_mac_failure_total` incremented.

### MCN-503 PIN and MAC verification (issuer)
Lane ISS · 5 pts · Depends: MCN-501 · Traces: FR-11
As an Issuer, I want to verify PIN and MAC, so that stolen cards and tampered messages are rejected.
1. PVV verification; wrong PIN ⇒ RC 55 and `pin_try_count++`; third failure ⇒ status PIN_BLOCKED and RC 75.
2. Bad MAC ⇒ RC 96, no posting.
3. DE 52 is removed from the context after verification and never logged.

### MCN-504 Dynamic key exchange
Lane GW + ISS · 5 pts · Slice S5 · Depends: MCN-502, MCN-503 · Traces: FR-12
As a Security officer, I want to rotate the ZPK without downtime, so that key exposure is limited.
1. `POST /v1/keys/acquirer/rotations` runs GENERATE → SEND_0800_161 → PARTNER_CONFIRM → ACTIVATE with persisted steps.
2. Both sides accept the old key for 5 minutes after activation; transactions during rotation succeed (integration test at 50 TPS).
3. Key changes write audit records.

### MCN-505 Security and Keys screen
Lane WEB · 5 pts · Slice S5 · Depends: MCN-003 (mocks) · Traces: FR-11, FR-12
As a Learner, I want to see keys and how a PIN is protected, so that I understand the key hierarchy.
1. Key table with KCV, lifetime bar and warning for keys near expiry; rotation stepper driven by the rotation resource; KCV flips when a new key activates.
2. PIN block visualiser computes format 0 in the browser (illustration only, clearly labelled), validates 4–12 digits with an inline error.
3. PCI "never do" list in Easy/Expert wording.

### MCN-506 PCI scanning in CI
Lane PLAT · 3 pts · Depends: MCN-005 · Traces: NFR-06
As a Security officer, I want automatic detection of card data leaks, so that mistakes never reach main.
1. `make pci-scan` scans logs from the E2E and chaos runs, fixtures (except the allow-listed simulator fixture), and a DB dump for Luhn-valid PANs, PIN block and track patterns.
2. CI fails on any finding; gitleaks runs for secrets.

---

## E6 — EMV and advanced transactions (Sprint 8)

### MCN-601 Pre-authorization and completion (issuer)
Lane ISS · 5 pts · Slice S6 · Depends: MCN-402 · Traces: FR-06
1. 0100 with DE 25 = 06 creates an ACTIVE hold; available decreases, ledger does not.
2. 0220 completion ≤ hold amount posts the journal for the final amount, releases the rest, answers 0230; > hold ⇒ recorded and flagged for reconciliation (v1).
3. `HoldExpiryJob` releases expired holds and emits `HoldExpired`.

### MCN-602 EMV field 55
Lane ISS · 5 pts · Depends: MCN-302 · Traces: FR-13
1. TLV parser supports tags listed in ISO spec §11, rejects malformed TLV with RC 30.
2. Simulated ARQC verification; ATC must increase (`emv_last_atc`), replay ⇒ RC 05.
3. 0210 returns ARPC in tag 91 for chip transactions.

### MCN-603 Advanced transaction endpoints (gateway)
Lane GW · 5 pts · Slice S6 · Depends: MCN-303 · Traces: FR-01, FR-06
1. Pre-auth, completion, refund, balance inquiry endpoints per spec; balance returned from DE 54.
2. Partial approval (RC 10) returns `approvedAmount`.

### MCN-604 POS and Cards extensions
Lane WEB · 3 pts · Slice S6 · Depends: MCN-003 (mocks) · Traces: FR-01
1. POS transaction-type selector (purchase, pre-auth, completion, refund, balance) with correct fields per type.
2. Cards screen shows holds with expiry and completion state.

---

## E7 — Settlement (Sprint 9)

### MCN-701 Outbox relays
Lane ISS + GW (parallel) · 3 pts · Depends: MCN-302, MCN-303 · Traces: FR-15
1. Relay publishes unpublished outbox rows in order to Kafka with key RRN and headers `traceparent`, `event-id`, `event-type`; marks published after ack.
2. `mcn_outbox_lag_seconds` < 2 s at 200 TPS.

### MCN-702 Cutover and 0500 reconciliation
Lane GW + ISS · 5 pts · Slice S7 · Depends: MCN-701 · Traces: FR-14
1. Scheduled and manual cutover sends 0800/201 with the new DE 15; in-flight messages keep their DE 15.
2. Gateway sends 0500 with totals; issuer answers 0510 RC 00 or 95 and stores both sides in `recon_totals`.

### MCN-703 Settlement ingestion
Lane SET · 5 pts · Depends: MCN-701 · Traces: FR-15
1. Consumers for both topics use the inbox pattern and ack after commit.
2. `clearing_record` holds the final state per source; out-of-order reversal before original is handled.

### MCN-704 Reconciliation, breaks and clearing file
Lane SET · 8 pts · Slice S7 · Depends: MCN-703 · Traces: FR-15
1. `reconcile()` is a pure function covering all `BreakType`s with table-driven tests.
2. Breaks can be resolved with a reason (audited); totals turn to "match" when all breaks are resolved.
3. Clearing file generation refused with 409 `open-breaks`; otherwise deterministic CSV + trailer + SHA-256 and net position.

### MCN-705 Settlement screen
Lane WEB · 5 pts · Slice S7 · Depends: MCN-003 (mocks) · Traces: FR-14, FR-15
1. Four-step wizard driven by `SettlementDay.stage`; totals table with match badges; breaks with resolve actions; file card.
2. Trying to generate the file with open breaks shows the API problem detail inline (shake).

---

## E8 — Switch and stand-in (Sprint 10)

### MCN-801 Switch with BIN routing
Lane GW · 5 pts · Depends: MCN-303 · Traces: FR-16
1. `cmd/switch` sits between gateway and issuer; routes by BIN table; gateway re-points to the switch via config only.
2. Two issuer connections pooled; responses may return on either.

### MCN-802 Circuit breaker and STIP
Lane GW · 8 pts · Slice S8 · Depends: MCN-801 · Traces: FR-16, NFR-04
1. Breaker thresholds per `docs/02` §7.4; states exposed on `/v1/network/switch` and WS.
2. While OPEN: approve ≤ STIP limit with STIP rules, decline others with RC 91; create 0120/0220 advices in the switch SAF.
3. On recovery, advices drain and the issuer ledger matches STIP approvals (integration test).

### MCN-803 Velocity rules
Lane ISS · 3 pts · Depends: MCN-302 · Traces: FR-17
1. Count and amount limits per card per day using `velocity_counter`; exceed ⇒ RC 65 or 61.
2. Rules are strategies registered in config; adding a rule needs no change to `CheckLimits`.

### MCN-804 Network screen v2
Lane WEB · 3 pts · Slice S8 · Depends: MCN-205 · Traces: FR-16
1. Topology adds the switch; breaker pills (CLOSED/OPEN/HALF_OPEN), STIP counters, SAF list.
2. "Simulate issuer down" drives the chaos scenario and the screen reacts live.

---

## E9 — Operations and learning (Sprint 11)

### MCN-901 Dashboards, alerts, runbooks
Lane PLAT · 5 pts · Traces: NFR-08
1. Grafana dashboards: traffic, latency, RC mix, reversals, SAF, links, outbox lag, ledger discrepancy.
2. Alerts: link down > 30 s, SAF DEAD > 0, discrepancy ≠ 0, p99 > 300 ms for 5 min, outbox lag > 10 s.
3. Runbooks in `docs/runbooks/` for each alert.

### MCN-902 Graceful drain everywhere
Lane ISS, GW, SET · 3 pts · Traces: NFR-09
1. SIGTERM under load: no transaction lost, SAF persisted, sign-off sent, exit ≤ 30 s (test per service).

### MCN-903 Load test and report
Lane PLAT · 3 pts · Traces: NFR-01
1. Load tool drives 200 TPS for 10 min; report p50/p95/p99 per hop and resource usage; stored as CI artifact.

### MCN-904 Lesson mode, search, English UI
Lane WEB · 5 pts · Traces: FR-18
1. Guided lessons (one per epic) that drive the UI step by step; progress stored locally.
2. ⌘K search by RRN, last 4 digits, TID.
3. English locale complete; language switch in settings.

### MCN-905 End-to-end suite
Lane WEB · 3 pts · Traces: all
1. Playwright covers the six critical journeys in `docs/08` §5 against `make up`, animations disabled, runs nightly and on release branches.
