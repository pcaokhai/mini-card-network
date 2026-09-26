# Data Model

Version 1.0 · 2026-09-21 · Baseline DDL: [`assets/baseline-schema.sql`](assets/baseline-schema.sql)

## 1. Ownership

One PostgreSQL instance, three databases, one role each. A service connects only to its own database (ADR-004).

| Database | Owner service | Migration tool | Location |
| --- | --- | --- | --- |
| `issuer` | issuer-jpos | Flyway | `issuer-jpos/src/main/resources/db/migration/V<n>__<desc>.sql` |
| `acquirer` | gateway-go (also switch tables from Sprint 10) | goose | `gateway-go/migrations/<n>_<desc>.sql` |
| `settlement` | settlement | Flyway | `settlement/src/main/resources/db/migration/V<n>__<desc>.sql` |

The baseline file shows all three schemas together for review. MCN-301 (issuer), MCN-303 (acquirer) and MCN-703 (settlement) split it into the first migration of each service and add the deltas in §4.

## 2. Tables per schema

| Schema | Table | Purpose | Key constraints |
| --- | --- | --- | --- |
| issuer | `account` | Ledger and available balance, overdraft | Floor `available ≥ −overdraft_limit` enforced by Authorize for debits under the row lock (the `chk_available_floor` CHECK was dropped in V7: a reversal always posts and may overdraw, flagged by an `audit_log` `NEGATIVE_BALANCE_AFTER_REVERSAL` row); `version` bumped on every change |
| issuer | `card` | Card status, encrypted PAN, HMAC, PVV, ATC | `pan_hash` unique; no CVV/PIN columns |
| issuer | `card_limit`, `velocity_counter` | Limits and fast counters | PK (card, type, period[, key]) |
| issuer | `tran_log` | Every ISO request/response at the issuer | Partitioned by `business_date`; `uq_tran_dedupe`; FK to original |
| issuer | `auth_hold` | Pre-auth and holds | One per transaction; partial index on ACTIVE expiry |
| issuer | `journal_entry`, `ledger_posting`, `gl_account` | Double-entry ledger | Deferred trigger Σ D = Σ C |
| issuer | `key_store` | Keys under LMK + KCV | One ACTIVE per (type, counterparty) |
| issuer | `acquirer_link`, `system_state`, `cutover_log`, `recon_totals` | Link state, business date, 0500 totals | |
| issuer | `outbox_event`, `audit_log` | Events to Kafka; append-only audit | Trigger forbids UPDATE/DELETE on audit |
| acquirer | `merchant`, `terminal` | POS master data | |
| acquirer | `tran_log`, `tran_state_history` | Transactions and state machine history | Partitioned; `uq_acq_client_req`, `uq_acq_network_key` |
| acquirer | `saf_queue` | Durable advices | Partial index on due PENDING rows |
| acquirer | `link_state`, `key_store`, `settlement_batch` | Link, keys, day batch | |
| settlement | `inbox_event` | Consumer idempotency | PK `event_id` |
| settlement | `clearing_record` | Final state per transaction per source | Unique per (source, date, acq, TID, STAN, F7) |
| settlement | `recon_run`, `recon_break`, `net_position`, `clearing_file` | Reconciliation and output | |

## 3. Modeling rules

- **Money:** `BIGINT` minor units + `CHAR(3)` numeric currency. No `NUMERIC`/`DOUBLE` for amounts.
- **Statuses:** `TEXT` + `CHECK` (not PostgreSQL ENUM) so values can be added in a migration without locking.
- **`tran_log` RECEIVED → outcome.** A row takes its outcome only while it is `RECEIVED` (a conditional `UPDATE`). An approval writes `APPROVED` in the same transaction that moves the money. A reversal that finds an original still `RECEIVED` longer than the acquirer's response timeout plus a margin (`stale-received-after-seconds`, default 60 s; docs/03 §9 is 30 s), and with no journal, abandons it as `REVERSED` (`decline_reason` 'abandoned: reversed while still RECEIVED') and acknowledges the 0420 with no ledger effect. No separate "abandoned" status: `REVERSED` already means the acquirer's reversal was acknowledged and nothing is owed, and it makes a late approval for that row roll back and decline RC 94 (docs/03 §7.3). A fresher `RECEIVED` original still answers 96 so the SAF repeats (#119).
- **Time:** `TIMESTAMPTZ` for instants (UTC), `DATE` for business dates, raw `CHAR(10)` for ISO DE 7 (it has no year).
- **Partitioning:** `tran_log` tables by month on `business_date`. Every PK/unique constraint includes `business_date` (PostgreSQL requirement). A monthly job (or pg_partman) creates partitions 2 months ahead; missing partition ⇒ alert.
- **Card data:** `pan_enc` (AES-GCM), `pan_hash` (HMAC-SHA256, separate key), `masked_pan`. Never add columns for CVV, PIN, PIN block, track data, or clear keys.
- **Audit:** append-only tables are protected by triggers; application roles have no UPDATE/DELETE grants on them.
- **IDs:** internal `BIGINT` identities; external references are opaque (`cardRef`, RRN). Never expose internal IDs in APIs.
- **Naming:** `snake_case`, singular table names, `ix_`/`uq_`/`fk_`/`chk_` prefixes, `created_at`/`updated_at` on mutable tables.

## 4. Deltas from the baseline (apply in the first migrations)

| Schema | Change | Reason | Story |
| --- | --- | --- | --- |
| issuer | `tran_log.trace_id VARCHAR(32)` | Cross-host tracing by RRN | MCN-302 |
| issuer | `tran_log.stored_response JSONB` | Byte-exact duplicate replay (DE 38, 39, 4, 54) | MCN-402 |
| issuer | `tran_log.status` add `REVERSAL_WITHOUT_ORIGINAL` | Reversal before original (ISO §7.3) | MCN-402 |
| issuer | `idempotency_record(key, route, request_hash, status, body, created_at)` | Admin API idempotency | MCN-308 |
| issuer | V7: drop `account.chk_available_floor`; add `tran_log.balance BIGINT NULL` | A reversal is an advice and must post even past the overdraft floor (the floor moved into Authorize's debit path); a duplicate balance inquiry replays its DE 54 | #119 (POS-G17/G19) |
| issuer | V8: unique index `ux_key_store_one_active (key_type, COALESCE(counterparty, '')) WHERE status = 'ACTIVE'` | The MAC/PIN participants read the one ACTIVE key and seed it from the environment at startup; concurrent seeding must not create two | SEC-G15 |
| acquirer | `outbox_event` (same shape as issuer) | Acquirer events for settlement | MCN-703 |
| acquirer | `idempotency_record` | REST idempotency | MCN-303 |
| acquirer | `tran_log.approved_amount`, `balance_amount`, `balance_currency`, `original_rrn`, `trace_id`, `reversal_reason` (migration 00010) | Transaction detail, W3C trace id, the 0420's DE 39 reason for `reversalReason` | POS-G8, JRN-G3, JRN-G7 |
| acquirer | `idempotency_record.rrn`, `tran_log.completed_by` (migration 00011) | The RRN a reserved key sent, so a retry answers from `tran_log`; the completion that consumed a pre-auth | #117 review S1, S2 |
| acquirer | `network_event(id, occurred_at, severity, easy_text, technical_text)` | Network screen timeline | MCN-204 |
| acquirer | `chaos_run(id, status, requested, completed, result JSONB)` | Chaos run tracking | MCN-404 |
| acquirer | `key_rotation(id, key_type, status, steps JSONB)` | Rotation workflow | MCN-504 |
| settlement | `settlement_day(business_date PK, stage, updated_at)` | Wizard stage | MCN-704 |

## 5. Migration rules

1. Forward-only; never edit an applied migration. Fixes are new migrations.
2. Expand → migrate → contract for breaking changes (add column nullable, backfill, then enforce/drop in a later release).
3. Each migration is idempotent-safe under Testcontainers from empty and is tested in CI by applying all migrations then running repository tests.
4. Reserve the migration number in the story plan; one migration per story unless the plan says otherwise.
5. No data changes in schema migrations except seed/reference data (`response_code`, `gl_account`). Lab seed data comes from `contracts/fixtures/` via a separate seed command.

## 6. Key queries (must be index-backed)

| Query | Index |
| --- | --- |
| Find original for reversal by F90 | `ix_tran_orig_match (acquirer_id, stan, transmission_dt_raw)` |
| Dedupe on insert | `uq_tran_dedupe` |
| Card history newest first | `ix_tran_card_time (card_id, created_at DESC)` |
| Lookup by RRN | `ix_tran_rrn`, `ix_acq_rrn` |
| Due SAF rows | `ix_saf_due (next_retry_at) WHERE status='PENDING'` |
| Unpublished outbox | `ix_outbox_unpublished (created_at) WHERE published_at IS NULL` |
| Expired holds | `ix_hold_expiry (expires_at) WHERE status='ACTIVE'` |
| Reconciliation matching | `uq_clearing` + `ix_clearing_match (business_date, rrn)` |
