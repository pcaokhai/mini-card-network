# API Contract

Version 1.0 · 2026-09-21 · Normative sources: `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `contracts/events/*.schema.json`

This document explains conventions and lists the endpoints. The files in `contracts/` are the source of truth; code is generated from them (Go strict server, TS client, MSW mocks).

Per-screen integration guides (which UI element reads which field, cadence, provider rules, known gaps): [Overview](api/overview-page.md).

## 1. Topology

The browser calls only the Next.js BFF (`/api/...`), which forwards to three backends with the same paths under `/v1`:

| Prefix | Backend | Base URL (compose) |
| --- | --- | --- |
| `/v1/transactions`, `/v1/lab`, `/v1/chaos`, `/v1/network`, `/v1/terminals`, `/v1/keys/acquirer`, `/v1/metrics`, `/v1/stream` | gateway-go | `http://gateway:8080` |
| `/v1/cards`, `/v1/accounts`, `/v1/keys/issuer` | issuer-jpos Admin API | `http://issuer:8081` |
| `/v1/settlement` | settlement | `http://settlement:8082` |

WebSocket: the browser connects directly to `ws://localhost:8080/v1/stream?token=…` with a 60-second token issued by `POST /api/stream-token` (BFF).

## 2. Conventions

| Topic | Rule |
| --- | --- |
| Versioning | URL major version `/v1`. Additive changes only within v1; breaking change ⇒ `/v2` + ADR |
| Media type | `application/json; charset=utf-8`; errors `application/problem+json` |
| Naming | JSON `camelCase`; enum values `SCREAMING_SNAKE_CASE`; resource paths plural kebab-case |
| IDs | Opaque strings. Transactions addressed by `rrn`; cards by `cardRef` (never PAN) |
| Money | `{ "amount": 250000, "currency": "704" }`, integer minor units, currency ISO 4217 numeric |
| Time | RFC 3339 UTC (`2026-09-21T07:32:08Z`); business dates `YYYY-MM-DD` |
| Idempotency | `Idempotency-Key` (UUID) REQUIRED on every POST/PUT/PATCH/DELETE that changes state; replay returns the stored status and body for 24 h; key reuse with a different body ⇒ 422 `idempotency-key-mismatch` |
| Concurrency | Updatable admin resources return `ETag`; updates require `If-Match`; mismatch ⇒ 412 |
| Pagination | Cursor based: `?limit=50&cursor=…` → `{ "items": [...], "nextCursor": "…" | null }`; max limit 200 |
| Filtering | Explicit query params (`status`, `from`, `to`, `rc`, `last4`); no generic query language |
| Tracing | `traceparent` accepted and propagated; responses include `X-Trace-Id` |
| Actor | BFF sends `X-Actor: <username>` for audit (no end-user auth in v1) |
| Display modes | APIs return both codes and plain-language labels where helpful (`responseCode`, `responseLabel`); the UI chooses what to show |
| Card data | Responses contain `maskedPan` (first 6 + last 4) at most; never PAN, CVV, PIN, PIN block, track data, clear keys |

## 3. Error model (RFC 9457)

```json
{
  "type": "https://mcn.local/problems/insufficient-idempotency-key",
  "title": "Idempotency-Key header is required",
  "status": 400,
  "detail": "POST /v1/transactions/purchases requires Idempotency-Key.",
  "instance": "/v1/transactions/purchases",
  "traceId": "4bf92f3577b34da6a3ce929d0e0e4736",
  "errors": [{ "field": "amount.amount", "message": "must be > 0" }]
}
```

| `type` suffix | Status | When |
| --- | --- | --- |
| `validation-error` | 400 | Body/params invalid (`errors[]` filled) |
| `insufficient-idempotency-key` | 400 | Missing header |
| `not-found` | 404 | Resource does not exist |
| `precondition-failed` | 412 | ETag mismatch |
| `conflict` | 409 | State transition not allowed (e.g. unblock an expired card) |
| `open-breaks` | 409 | Clearing file requested while breaks are open |
| `idempotency-key-mismatch` | 422 | Same key, different request |
| `link-down` | 503 | ISO link not signed on (transaction still recorded as declined RC 91) |
| `internal` | 500 | Unexpected; body contains only `traceId` |

A **declined or timed-out transaction is not an HTTP error**: it returns 201 with the transaction resource (`status: DECLINED | TIMED_OUT | REVERSAL_PENDING`) so the POS can show the result.

## 4. Endpoint catalogue

| Method | Path | Story | Purpose |
| --- | --- | --- | --- |
| POST | `/v1/transactions/purchases` | MCN-303 | Purchase (0200). Waits up to the ISO timeout; returns final or `REVERSAL_PENDING` state |
| POST | `/v1/transactions/pre-authorizations` | MCN-603 | Pre-auth (0100, DE 25 `06`) |
| POST | `/v1/transactions/{rrn}/completions` | MCN-603 | Completion advice (0220) |
| POST | `/v1/transactions/refunds` | MCN-603 | Refund (0200, DE 3 `20`) |
| POST | `/v1/transactions/balance-inquiries` | MCN-603 | Balance (0200, DE 3 `31`) |
| POST | `/v1/transactions/{rrn}/cancellations` | MCN-401 | POS cancel ⇒ reversal (DE 39 `17`) |
| GET | `/v1/transactions` | MCN-304 | List with filters, cursor pagination |
| GET | `/v1/transactions/{rrn}` | MCN-304 | Detail |
| GET | `/v1/transactions/{rrn}/journey` | MCN-304 | Ordered journey steps incl. messages (masked) and money timeline. Each step carries a language-neutral `code` (`StepCode`) that clients render their own copy from |
| GET | `/v1/terminals` | MCN-305 | Terminals and merchants for the POS |
| POST | `/v1/lab/messages/decode` | MCN-103 | Hex/ASCII → fields + bitmap breakdown |
| POST | `/v1/lab/messages/encode` | MCN-103 | Fields → packed message |
| GET | `/v1/lab/messages/samples` | MCN-103 | Sample messages per MTI |
| GET | `/v1/network/links` | MCN-204 | Link status, last echo, latency, in-flight |
| POST | `/v1/network/links/{linkId}/echo` | MCN-204 | Send 0800/301 now |
| POST | `/v1/network/links/{linkId}/sign-on` · `/sign-off` | MCN-204 | Manual sign-on/off |
| GET | `/v1/network/saf` | MCN-401 | SAF depth + items (masked) |
| GET | `/v1/network/events` | MCN-204 | Operational events, cursor paginated |
| GET | `/v1/network/switch` | MCN-802 | Circuit breaker + STIP status |
| GET, PUT | `/v1/chaos/scenarios` · `/{scenarioId}` | MCN-404 | List / toggle chaos scenarios |
| POST | `/v1/chaos/runs` | MCN-404 | Run N synthetic transactions; returns run id |
| GET | `/v1/chaos/runs/{runId}` | MCN-404 | Progress + money verification result |
| GET | `/v1/metrics/overview` | MCN-306 | KPIs, throughput series, decline reasons |
| GET | `/v1/cards` | MCN-308 | Cards (masked) with status |
| GET | `/v1/cards/{cardRef}` | MCN-308 | Card, balances, holds, limits, usage |
| POST | `/v1/cards/{cardRef}/blocks` · `DELETE` same | MCN-308 | Block / unblock (audited) |
| PUT | `/v1/cards/{cardRef}/limits` | MCN-308 | Update limits (If-Match) |
| GET | `/v1/cards/{cardRef}/ledger` | MCN-308 | Journal entries with postings |
| GET | `/v1/keys/issuer` · `/v1/keys/acquirer` | MCN-501 | Key inventory (type, KCV, status, expiry) |
| POST | `/v1/keys/acquirer/rotations` | MCN-504 | Start ZPK rotation; returns rotation id |
| GET | `/v1/keys/acquirer/rotations/{rotationId}` | MCN-504 | Rotation steps and status |
| GET | `/v1/settlement/days/{businessDate}` | MCN-705 | Stage, totals comparison, net position |
| POST | `/v1/settlement/days/{businessDate}/cutover` | MCN-702 | Trigger cutover (gateway sends 0800/201) |
| POST | `/v1/settlement/days/{businessDate}/reconciliations` | MCN-704 | Run reconciliation |
| GET | `/v1/settlement/days/{businessDate}/breaks` | MCN-704 | Breaks list |
| POST | `/v1/settlement/breaks/{breakId}/resolutions` | MCN-704 | Resolve a break (audited) |
| POST | `/v1/settlement/days/{businessDate}/clearing-files` | MCN-704 | Generate clearing file (409 if open breaks) |

Full schemas: `contracts/openapi.yaml`. The PIN-block visualizer on the Security screen is computed client-side for illustration and has no endpoint.

## 5. WebSocket events

Endpoint `GET /v1/stream` (upgrade). Server → client only. Envelope:

```json
{ "id": "01J8Z…", "type": "transaction.updated", "occurredAt": "2026-09-21T07:32:08.182Z", "traceId": "…", "data": { } }
```

| `type` | `data` | Emitted when |
| --- | --- | --- |
| `transaction.created` | TransactionSummary | Gateway accepts a POS request |
| `transaction.updated` | TransactionSummary + `previousStatus` | Any state transition |
| `link.status` | Link | Link up/down/sign-on/echo result |
| `saf.changed` | `{ depth, deadCount }` | SAF depth changes (coalesced, max 2/s) |
| `network.event` | NetworkEvent | Operational event appended |
| `chaos.changed` | ChaosScenario | Scenario toggled |
| `chaos.run.progress` | ChaosRunProgress | Every 500 ms during a run |
| `switch.status` | SwitchStatus | Circuit breaker state change |
| `heartbeat` | `{}` | Every 15 s |

Client rules: reconnect with backoff 1–30 s; on reconnect refetch lists (events are not replayed); ignore unknown `type`; validate `data` with generated Zod schemas. Schema: `contracts/ws-events.schema.json`.

## 6. Kafka events

Topics `issuer.transactions.v1` and `acquirer.transactions.v1`, key = RRN, headers `traceparent`, `event-id`, `event-type`. Event types: `TransactionApproved`, `TransactionDeclined`, `TransactionReversed`, `HoldCreated`, `HoldCompleted`, `HoldExpired`, `BusinessDateClosed`. Schemas in `contracts/events/`. Producers use the outbox; consumers are idempotent by `event-id`.

## 7. Parallel development workflow

1. Contract PR: update `openapi.yaml`/schemas + example fixtures in `contracts/fixtures/`. CI runs spectral lint and `oasdiff` breaking-change check.
2. After merge: GW regenerates the strict server (compile errors show unimplemented handlers); WEB regenerates types and MSW handlers and builds screens against fixtures.
3. Backend contract tests validate real responses against the spec; frontend tests run against MSW using the same fixtures.
4. Integration checkpoint at slice end: WEB flips `NEXT_PUBLIC_API_MOCKS=false`, runs Playwright against `make up`.
