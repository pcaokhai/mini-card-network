# Chaos Lab page: API contract

| | |
| --- | --- |
| Document | `docs/api/chaos-lab-page.md` |
| Version | 1.0 |
| Status | Approved for integration |
| Date | 2026-09-25 |
| Screen | route `/lab/chaos`, container `web-next/src/app/(console)/lab/chaos/ChaosLabScreen.tsx` |
| Stories | MCN-405 (WEB), MCN-404 (GW) |
| Provider(s) | gateway-go (`internal/api/chaos.go`, `internal/chaos/toxiproxy_client.go`, `internal/chaos/scenario.go`, `internal/chaos/runner.go`, `internal/ws/hub.go`); Toxiproxy admin API behind it |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/chaos-client.ts` (plus `network-client.ts` and `overview-client.ts` for the reaction tiles); WS through `src/shared/ws/useWsEvents.ts` |
| Design reference | canvas `project/ChaosLab.dc.html` (docs/01-prd.md §7.1); plan `docs/plans/MCN-405-chaos-canvas.md`; release notes `docs/releases/R5.3.md` "Phòng lab sự cố" |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only). `PUT /v1/chaos/scenarios/*` and `POST /v1/chaos/runs` were **not** called on the shared stack; their examples come from dev:mock (`web-next/src/mocks/pages/chaos.ts`) or from the provider code, labelled as such |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and are not repeated.

## 1. Purpose and scope

The Chaos Lab lets a viewer switch six failure scenarios on and off, fire a batch of 100 synthetic purchases through the real authorization path, and watch a money-verification panel prove that no money was lost. A reaction panel explains what the system does for each active failure. This contract covers scenario listing and toggling, starting and reading a run, the WS events `chaos.changed` and `chaos.run.progress`, and the two reaction tiles the page borrows from other endpoints.

Out of scope: the chaos test suite (MCN-407, `make chaos`); the Network page's issuer-down toggle, which uses the same PUT (see [network-page.md](network-page.md) §4.10); and a list of past runs, for which **no endpoint exists** (plan Ruling R2).

## 2. Page map

| UI region (canvas / i18n label) | Data shown | Call(s) (§4.x) | Refresh |
| --- | --- | --- | --- |
| Title "Phòng thí nghiệm sự cố" + subtitle | static copy | none | none |
| Pill "Mọi thứ bình thường" / "{count} sự cố đang bật" | count of `enabled` scenarios | §4.1 | on mount, after every toggle, on WS `chaos.changed` |
| Button "Tắt tất cả" | one PUT `enabled=false` per active scenario | §4.2 (× n) | click |
| Button "Chạy thử 100 giao dịch" | starts a run of 100; disabled while a run is RUNNING/VERIFYING or the POST is pending (Ruling R7) | §4.3 | click |
| Alert line | "Không đổi được sự cố. Hãy thử lại." / "Không bắt đầu được lần chạy thử. Hãy thử lại." (Ruling R8) | §4.2, §4.3 errors | on failure |
| Six scenario cards (titles "Mạng chậm", "Mất kết nối", "Mất câu trả lời", "Gửi trùng", "Ngân hàng phát hành sập", "Câu trả lời đến muộn") | title, description, Expert tech line, toggle "Bật sự cố" / "Đang bật · bấm để tắt", amber when on | §4.1 `enabled`; copy from `chaos.scenarios.{id}` (Ruling R1) | as the pill |
| "Kiểm chứng tiền" | badge "Chưa chạy thử" / "Đang kiểm chứng…" / "Sổ sách khớp" / "Sổ sách lệch"; five ledger rows; "Lệch trong lần chạy {runId}" | §4.4, WS `chaos.run.progress` | 2 s poll while running + WS |
| "Hệ thống đang phản ứng thế nào": tile "Lệnh đang chờ gửi lại" / "SAF depth" | SAF depth | §4.5 | 5 s poll |
| same: tile "Thời gian phản hồi" / "p99 end-to-end" | overview p99 ("212 ms" or "3,2 giây") | §4.6 | 30 s poll |
| same: reaction list | one item per active scenario, in canvas order: title + `react` (Easy) or `reactTech` (Expert); calm text "Mọi thứ đang bình thường…" when none | §4.1 | as the pill |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency (If-Match/ETag) | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/chaos/scenarios` | gateway-go (reads Toxiproxy) | Six scenarios and their state | on mount; after every PUT; on WS `chaos.changed` | n/a | none | Real |
| 4.2 | `PUT /v1/chaos/scenarios/{scenarioId}` | gateway-go (writes Toxiproxy) | Enable / disable one scenario | card click; "Tắt tất cả" (one per active scenario, in parallel) | required, fresh UUID per PUT; presence checked only, not deduplicated | none (last write wins) | Real; `DROP_RESPONSE` enable is 501 unless `CHAOS_FAKE_ISSUER_ADDR` is set |
| 4.3 | `POST /v1/chaos/runs` | gateway-go | Start a synthetic run of N purchases | click "Chạy thử 100 giao dịch" | required, fresh UUID per click; presence checked only, not deduplicated | none | Real |
| 4.4 | `GET /v1/chaos/runs/{runId}` | gateway-go | Run progress and verdict | every 2 s until PASSED/FAILED | n/a | none | Real (in-memory; lost on gateway restart) |
| – | `GET /v1/chaos/runs` (list) | – | Recover the latest run after reload | – | – | – | **Does not exist** (Ruling R2, CHA-G6) |
| 4.5 | `GET /v1/network/saf` | gateway-go | SAF depth tile | every 5 s | n/a | none | Real, see [network-page.md](network-page.md) §4.5 |
| 4.6 | `GET /v1/metrics/overview` | gateway-go | p99 tile (`p99LatencyMs`) | every 30 s | n/a | none | Real, see [overview-page.md](overview-page.md) |
| 5.1 | WS `chaos.changed` | gateway-go | Scenario toggled; also sent at run end | push | – | – | Real (two payload shapes, CHA-G2) |
| 5.2 | WS `chaos.run.progress` | gateway-go | Run snapshot after each purchase | push | – | – | Real |
| – | Client-only data | web-next | Titles, descriptions, tech lines and reactions per scenario (`chaos.scenarios.*` in `messages/vi.json`), icons (`SCENARIO_ICONS`), run size 100 | static | – | – | Real |

## 4. Calls

### 4.1 GET /v1/chaos/scenarios

- **Summary.** Returns the six scenarios in fixed order with their current state.

**Request.** No parameters.

**Response.** `200 OK`, array of `ChaosScenario`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `id` | `ChaosScenarioId` | ✓ | card key; copy key `chaos.scenarios.{id}.*`; icon | title + `desc` | title + `desc` + `tech` line |
| `enabled` | boolean | ✓ | card highlight, `aria-pressed`, toggle label; pill count; reaction list | "Đang bật · bấm để tắt" / "Bật sự cố" | same |
| `easyText` | string (English) | ✓ | not shown (Ruling R1) | – | – |
| `technicalText` | string (English) | ✓ | not shown (Ruling R1) | – | – |

Scenario catalogue: provider mechanism (`internal/chaos/toxiproxy_client.go` `specFor`, `internal/chaos/scenario.go`) against page copy (`messages/vi.json`).

| `id` | Provider mechanism | Card title | Expert tech line shown |
| --- | --- | --- | --- |
| `SLOW_NETWORK` | Toxiproxy `latency` toxic, downstream, 3000 ms, jitter 0 | "Mạng chậm" | "Toxiproxy latency 3000 ms ± 200" (jitter is 0, CHA-G9) |
| `CONNECTION_CUT` | Toxiproxy `reset_peer` toxic, downstream, timeout 0 | "Mất kết nối" | "Toxiproxy reset_peer · link DOWN" |
| `DROP_RESPONSE` | not a toxic: repoints the issuer proxy upstream at the fake issuer (`CHAOS_FAKE_ISSUER_ADDR`), which accepts and never replies; without that env var, enable gives 501 | "Mất câu trả lời" | "Drop 0210 → timeout → 0420" |
| `DUPLICATE_REQUEST` | not a toxic: in-process flag; `purchase.Service` sends a blocking duplicate 0200 before the real send | "Gửi trùng" | "Duplicate 0200 · cùng STAN + field 7" |
| `ISSUER_DOWN` | Toxiproxy `reset_peer` toxic, downstream, timeout 0 (same effect as `CONNECTION_CUT`) | "Ngân hàng phát hành sập" | "Issuer core down → STIP" (STIP isn't built, CHA-G9) |
| `LATE_RESPONSE` | Toxiproxy `latency` toxic, downstream, 35000 ms (over the 30 s request timeout, docs/03 §9) | "Câu trả lời đến muộn" | "Delay 0210 > timeout → late response" |

**Provider rules.**
1. The response always holds exactly six items in the order `SLOW_NETWORK`, `CONNECTION_CUT`, `DROP_RESPONSE`, `DUPLICATE_REQUEST`, `ISSUER_DOWN`, `LATE_RESPONSE` (`chaos.AllScenarios`).
2. For toxic-backed scenarios, `enabled` is true when a toxic named `chaos-{id}` exists on the issuer proxy (read live from Toxiproxy on every call). For `DROP_RESPONSE` and `DUPLICATE_REQUEST`, it is the gateway's in-memory flag.
3. State is global to the gateway process and shared by every viewer. At boot, the gateway disables all six scenarios (`DisableAll`).
4. `easyText` and `technicalText` are fixed English strings.
5. Client normalisation: the client maps the response onto its own six ids. A missing id renders as `enabled: false`, and duplicates collapse (`normalizeScenarios`).

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 500 | `chaos-list-failed` | Toxiproxy admin API unreachable or erroring | all six cards render off, pill "Mọi thứ bình thường" (misleading, CHA-G10); no alert |
| 502 | BFF `upstream-unavailable` | gateway unreachable | same |

**Example.** Real, local stack, 2026-09-25.

```json
[
  { "id": "SLOW_NETWORK", "enabled": false, "easyText": "The network gets slow.", "technicalText": "3000ms latency toxic on the issuer link." },
  { "id": "CONNECTION_CUT", "enabled": false, "easyText": "The connection drops mid-transaction.", "technicalText": "reset_peer toxic on the issuer link." },
  { "id": "DROP_RESPONSE", "enabled": false, "easyText": "The issuer's reply never arrives.", "technicalText": "Repoints the issuer link at a fake-issuer simulator that accepts the request and never replies." },
  { "id": "DUPLICATE_REQUEST", "enabled": false, "easyText": "The same payment is sent twice.", "technicalText": "The gateway sends the same 0200 twice with the same STAN." },
  { "id": "ISSUER_DOWN", "enabled": false, "easyText": "The issuer is completely unreachable.", "technicalText": "reset_peer toxic on the whole issuer link." },
  { "id": "LATE_RESPONSE", "enabled": false, "easyText": "The reply arrives after we've given up.", "technicalText": "latency toxic exceeding the 30s request timeout." }
]
```

**Notes.** Query key `["chaos","scenarios"]`, no polling, TanStack defaults (3 retries, refetch on focus). No `ETag`. Rate limit: not enforced.

### 4.2 PUT /v1/chaos/scenarios/{scenarioId}

- **Summary.** Enables or disables one scenario. **Destructive on a shared stack**: toxics act on the single issuer proxy that every user's traffic crosses.

**Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `scenarioId` | path | `ChaosScenarioId` | ✓ | contract enum of six; **not validated** by the provider (CHA-G4) | – |

| Header | Required | Notes |
| --- | --- | --- |
| `Idempotency-Key` | ✓ | the client sends a fresh UUID per PUT; a missing header gives 400. The key is not stored or replayed |
| `Content-Type` | ✓ | `application/json` |
| `traceparent` | no | forwarded by the BFF |

| Body field | Type | Required | Constraints | Notes |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | ✓ | – | a missing field decodes as `false`, which disables (CHA-G4) |

**Response.** `200 OK`, the updated `ChaosScenario` (§4.1 shape), read back through `ListScenarios`. The page doesn't read the body; it invalidates `["chaos","scenarios"]` on settle.

**Provider rules.**
1. Toggling is idempotent by state. Adding a toxic that already exists (Toxiproxy 409), or removing one that is gone (404), counts as success.
2. `DROP_RESPONSE` with `enabled: true` and no `CHAOS_FAKE_ISSUER_ADDR` gives 501 `scenario-not-implemented`. Disabling it always succeeds.
3. After a successful change, the gateway broadcasts WS `chaos.changed` with the same `ChaosScenario` it returns.
4. An unknown `scenarioId` isn't rejected: the provider records it in memory, answers **200** with a zero-value scenario (`"id": ""`), and broadcasts that (CHA-G4).
5. Concurrent PUTs aren't serialised; the last write wins per scenario. "Tắt tất cả" sends its PUTs in parallel.
6. The effects apply to live traffic at once (see the §4.1 catalogue). `LATE_RESPONSE` and `DROP_RESPONSE` make purchases time out after 30 s and queue 0420 reversals in SAF.

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `idempotency-key-required` | header missing (not reachable from the page) | "Không đổi được sự cố. Hãy thử lại." |
| 400 | `invalid-request` | body is not JSON | same |
| 501 | `scenario-not-implemented` | `DROP_RESPONSE` enabled without a fake issuer | same; the card stays off after the refetch |
| 500 | `chaos-set-failed` | Toxiproxy add/remove toxic or proxy update failed | same |
| 500 | `chaos-list-failed` | the read-back after the change failed; the change **was** applied | same, although the scenario did change (the refetch then shows it on) |
| 502 | BFF `upstream-unavailable` | gateway unreachable | same |

The alert stays until the next successful mutation of the same kind (TanStack `isError`).

**Example.** Not called on the shared stack. Request:

```http
PUT /api/v1/chaos/scenarios/SLOW_NETWORK
Idempotency-Key: <fresh UUID v4>
Content-Type: application/json

{ "enabled": true }
```

Response as the provider builds it (`scenario.go` text, `enabled` read back):

```json
{ "id": "SLOW_NETWORK", "enabled": true, "easyText": "The network gets slow.", "technicalText": "3000ms latency toxic on the issuer link." }
```

Mock (dev:mock, `mocks/pages/chaos.ts`): `{ "id": "SLOW_NETWORK", "enabled": true, "easyText": "SLOW_NETWORK", "technicalText": "SLOW_NETWORK" }`.

**Notes.** No retry (mutation). While a card's PUT is pending, that card is `aria-busy`. Rate limit: not enforced.

### 4.3 POST /v1/chaos/runs

- **Summary.** Starts a background run of N synthetic purchases on seed cards and returns at once with the initial snapshot. **It moves real ledger balances** on the shared stack.

**Request.**

| Header | Required | Notes |
| --- | --- | --- |
| `Idempotency-Key` | ✓ | a UUID, fresh per click; missing or not a UUID → 400 `insufficient-idempotency-key`. A retry with the same key and body **replays the first 202 body, the RUNNING snapshot**, so clients follow the run by polling `GET /v1/chaos/runs/{runId}`; the same key with a different body → 422 (#118) |
| `Content-Type` | ✓ | `application/json` |

| Body field | Type | Required | Constraints | Notes |
| --- | --- | --- | --- | --- |
| `transactions` | integer | ✓ | 1–10000, else 400 `validation-error` | the page always sends `100` |

**Response.** `202 Accepted`, `ChaosRun` (fields in §4.4), with `status: RUNNING`, `seq: 0`, `startedAt` set, all counts and totals 0 (the opening read happens after the 202). The client writes this body into the run's cache entry and starts polling §4.4.

**Provider rules.** (as of #118)
1. `runId` is `run-` + 16 lowercase hex characters (random). One run at a time: a second POST while one is RUNNING/VERIFYING → 409 `conflict`.
2. **The run assumes no other traffic on the seed cards while it runs.** A POS purchase or a stray reversal on those cards moves the issuer balances and shows up as `LEDGER_MISMATCH`; the mismatch's `failureDetail` says so.
3. Before the opening read, the run waits for the SAF queue to be empty (up to 65 s), so reversals left from earlier traffic can't land mid-run. Still pending → `FAILED`, `RUN_ERROR`, "SAF not drained before the run (N pending)".
4. `openingBalanceTotal` is the sum of the six seed cards' `ledgerBalance` from the Issuer Admin API (`GET {ISSUER_ADMIN_URL}/v1/cards/{cardRef}`, with the W3C `traceparent`). Cards in different currencies can't be summed → `RUN_ERROR`.
5. The run calls `purchase.Service.CreatePurchase` directly (not over HTTP), **sequentially**. Each purchase uses a random seed card, terminal `00000042`, entry mode `CHIP_PIN`, amount 10000 minor units in currency `704`, and idempotency key `chaos-{runId}-{i}`.
6. Outcome tally per purchase: `APPROVED` → `approved`; `DECLINED` (including RC 91 while the link is down) → `declined`; `TIMED_OUT` / `REVERSAL_PENDING`, and `DECLINED` with RC 96 (a response MAC failure, which the gateway reverses) → reversal candidate. `completed` goes up by one either way, and every snapshot goes out as `chaos.run.progress` with an increasing `seq`.
7. After the last purchase the run is `VERIFYING`: it waits for the SAF queue to drain (up to 65 s), counts the candidates whose `tran_log` status is `REVERSED` into `reversed`, and reads the closing balances. A drain timeout or an unreadable queue → `RUN_ERROR` "SAF not drained after the run …", never a ledger verdict.
8. `ledgerDiscrepancy = (closing − opening) − (−Σ approved)`. Zero → `PASSED`; otherwise `FAILED` with `LEDGER_MISMATCH`.
9. Any other failure (a purchase-service error, an issuer or database read failure, a panic, shutdown, or the wall-clock cap `CHAOS_RUN_MAX_DURATION`, default 10 min) ends the run `FAILED` with `RUN_ERROR` and a generic `failureDetail`; the raw error is logged, PAN-masked, with the run id and trace id.

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `idempotency-key-required` | header missing (not reachable from the page) | "Không bắt đầu được lần chạy thử. Hãy thử lại." |
| 400 | `invalid-request` | body is not JSON, or `transactions < 1` | same |
| 500 | `chaos-run-start-failed` | declared by the handler; `Runner.Start` never returns an error today | same |
| 502 | BFF `upstream-unavailable` | gateway unreachable | same |

**Example.** Not called on the shared stack. Request body `{ "transactions": 100 }`. Mock response (dev:mock, `mocks/pages/chaos.ts`):

```json
{
  "runId": "run-1",
  "status": "RUNNING",
  "requested": 100,
  "completed": 0,
  "approved": 0,
  "declined": 0,
  "reversed": 0,
  "openingBalanceTotal": 500000000,
  "closingBalanceTotal": 0,
  "ledgerDiscrepancy": 0
}
```

The real provider returns the same shape with `runId` like `run-3f9a21c07be45d18`.

**Notes.** No retry (mutation). Rate limit: not enforced.

### 4.4 GET /v1/chaos/runs/{runId}

- **Summary.** Current snapshot of one run.

**Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `runId` | path | string | ✓ | a `runId` from §4.3 | – |

**Response.** `200 OK`, `ChaosRun`. Before any run, every ledger row shows "—" and the badge reads "Chưa chạy thử".

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `runId` | string | ✓ | cache key; discrepancy line | "Lệch trong lần chạy {runId}" when FAILED | same |
| `status` | `RUNNING` \| `VERIFYING` \| `PASSED` \| `FAILED` | ✓ | badge; stops polling at PASSED/FAILED; run button disabled otherwise | "Đang kiểm chứng…" / "Sổ sách khớp" / "Sổ sách lệch" | same |
| `requested` | integer | ✓ | not shown | – | – |
| `completed` | integer | ✓ | not shown | – | – |
| `approved` | integer | – | label of row 2 | "{count} giao dịch được duyệt" | "Giao dịch RC 00 · {count} lệnh" |
| `declined` | integer | – | not shown | – | – |
| `reversed` | integer | – | row 3 value | label "Giao dịch đã tự hủy, không trừ tiền", value "{count} lệnh" | label "Đã reverse (0420 → 0430)", same value |
| `openingBalanceTotal` | integer (minor units) | – | row 1 value | label "Tổng số dư đầu ngày", value "500.000.000 ₫" | label "Số dư đầu ngày (opening)" |
| `closingBalanceTotal` | integer (minor units) | – | row 4 value, and row 2 value = −(opening − closing) | "Số dư hiện tại"; "…" until finished | "Σ available_balance" |
| `ledgerDiscrepancy` | integer (minor units), must be 0 | – | row 5 value; green flash when 0 and PASSED, red otherwise | "Chênh lệch"; "…" until finished | "Σ ledger − Σ tran_log" |

**Provider rules.**
1. Runs live in gateway memory: never evicted, and lost on restart (then 404).
2. `status` goes `RUNNING` → `VERIFYING` → `PASSED` or `FAILED` (a `RUN_ERROR` can end it from `RUNNING`). `seq` increases with every snapshot; a client drops a snapshot older than one it holds.
3. During `RUNNING`, `approved`, `declined` and `completed` grow. `reversed`, `closingBalanceTotal` and `ledgerDiscrepancy` are set during `VERIFYING` (§4.3 rule 7).
4. Amounts are integer minor units, currency `704` implied (the client formats with `704`).

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | `not-found` | unknown id, or the gateway restarted | query error; polling continues every 2 s with retries, and the panel keeps the last snapshot (CHA-G7) |
| 502 | BFF `upstream-unavailable` | gateway unreachable | same |

**Example.** Real error, local stack, 2026-09-25:

```json
{ "type": "not-found", "title": "not-found", "status": 404, "detail": "no such chaos run" }
```

Mock success (dev:mock, `DROP_RESPONSE` on, after 3.8 s):

```json
{
  "runId": "run-1",
  "status": "PASSED",
  "requested": 100,
  "completed": 100,
  "approved": 86,
  "declined": 6,
  "reversed": 8,
  "openingBalanceTotal": 500000000,
  "closingBalanceTotal": 484090000,
  "ledgerDiscrepancy": 0
}
```

**Notes.** Query key `["chaos","run", runId]`, enabled only when a run was started in this page instance. `refetchInterval` is 2000 ms until `PASSED`/`FAILED`, then off. Default retries (3). The `runId` lives in React state, so leaving the page or reloading loses the run (CHA-G6). Rate limit: not enforced.

### 4.5 GET /v1/network/saf

- **Summary.** Used only for `depth` on the tile "Lệnh đang chờ gửi lại" / "SAF depth" ("—" until loaded, vi-VN grouping). Full contract: [network-page.md](network-page.md) §4.5. It shares the query key `["network","saf"]` and the 5 s poll.

### 4.6 GET /v1/metrics/overview

- **Summary.** Used only for `p99LatencyMs` on the tile "Thời gian phản hồi" / "p99 end-to-end": "{value} ms" below 1000 ms, "{value} giây" with one decimal above ("3,2 giây"), "—" until loaded (Ruling R4). Full contract: [overview-page.md](overview-page.md). 30 s poll, so the tile lags a scenario change by up to 30 s.

## 5. Real-time events

Socket and client rules as in [network-page.md](network-page.md) §5: `NEXT_PUBLIC_WS_URL` or same-origin `/v1/stream`, backoff from 1 s to 30 s, no schema validation, and no `traceId` in the envelope. The page subscribes to `chaos.run.progress` and `chaos.changed`.

| Event | `data` schema (contract) | Emitted by (real) | UI effect | Ordering / dedupe |
| --- | --- | --- | --- | --- |
| `chaos.changed` | `ChaosScenario` | `PUT /v1/chaos/scenarios/{id}` after a successful change. Since #118 a run's final state goes out as `chaos.run.progress` instead (CHA-G2) | invalidates `["chaos","scenarios"]` | refetch, so no dedupe needed |
| `chaos.run.progress` | `ChaosRun` | the runner, after each purchase (not "every 500 ms" as docs/04 §5 says; it names a `ChaosRunProgress` schema that doesn't exist) | if `data.runId` equals the page's current run, `setQueryData(["chaos","run", runId], data)`; other runs are ignored | no sequence number. A late event can overwrite a newer polled snapshot until the next poll (at most 2 s) |

## 6. Security and compliance

| Topic | Rule on this page |
| --- | --- |
| Card data | No response carries a PAN or card token. Runs report counts and totals only. |
| Key material | None. |
| Destructive actions | Every scenario toggle acts on the one issuer proxy of the shared stack, and a run posts 100 real purchases (10000 minor units each) against the seed cards. Those change issuer balances, write `tran_log` rows, may queue reversals, and show up on every other screen. Safeguards today: none. No confirmation, no role check (v1 has no end-user auth), no guard against concurrent runs. The gateway clears all scenarios at boot. |
| Ledger integrity | Runs go through the normal purchase path, so double entry, SAF and idempotency apply (engineering rules 4 and 5). The run's verdict is not an independent check (CHA-G1). |
| Audit trail | Scenario toggles and run starts are not audited and not written to `network_event`. The BFF forwards no `X-Actor`. |
| Idempotency | `Idempotency-Key` must be a UUID. A retried PUT re-applies the same state (harmless, not stored). A retried POST with the same key and body replays the first 202, the RUNNING snapshot, and starts nothing; poll `GET /v1/chaos/runs/{runId}` for progress. Keys live in gateway memory (#118). |

## 7. Non-functional requirements

| Call | Latency budget | Observed on the local stack (2026-09-25) | Payload bound | Pagination |
| --- | --- | --- | --- | --- |
| §4.1 scenarios | < 300 ms (includes one Toxiproxy admin call) | p50 2.6 ms, p99 4.2 ms over 50 GETs (observed) | 6 items, 953 B | none |
| §4.2 toggle | < 1 s (two or three Toxiproxy admin calls) | not called (shared stack) | 1 item | – |
| §4.3 start | returns at once (202); run in background | not called | 1 item | – |
| §4.4 run | < 300 ms (in-memory) | 404 path < 2 ms (observed) | 1 item | none |
| Run duration | none defined | derived from the code: sequential purchases, so 100 × per-purchase latency plus up to 65 s of SAF drain. About 100 × 3 s ≈ 5 min under `SLOW_NETWORK`, about 100 × 30 s ≈ 50 min under `LATE_RESPONSE` or `DROP_RESPONSE`. Seconds when calm or with the link down (immediate RC 91 declines) | – | – |

Polling load while a run is active: §4.4 every 2 s, §4.5 every 5 s, §4.6 every 30 s. Memory: runs accumulate in the gateway process until restart.

## 8. UI states

| State | What renders | Driving call |
| --- | --- | --- |
| Loading | six cards off, pill "Mọi thứ bình thường", money panel "Chưa chạy thử" with "—" rows, tiles "—", calm reaction text | §4.1, §4.5, §4.6 pending |
| Empty (no run yet) | badge "Chưa chạy thử"; row labels without counts ("Giao dịch được duyệt"); every value "—" | no §4.4 query |
| Running | badge "Đang kiểm chứng…"; opening shown; approved count grows in the label; row 2, current and discrepancy values "…"; run button disabled | §4.4 poll + WS |
| Finished PASSED | badge "Sổ sách khớp" with a check; all rows filled; discrepancy "0 ₫" flashes green (once per run) | §4.4 |
| Finished FAILED | badge "Sổ sách lệch"; discrepancy in red; "Lệch trong lần chạy {runId}". On the real provider this means "a purchase call errored", not an actual ledger difference (CHA-G3) | §4.4 |
| Partial | the tiles fail on their own ("—"); a failing scenarios query shows every card off | §4.1, §4.5, §4.6 |
| Error | toggle or start failure: the alert line (§2); a GET failure has no message | §4.2, §4.3 |
| Provider not available | as Loading, plus alerts on any click | all |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| CHA-G1 | The money verification is tautological. `closingBalanceTotal` is derived from the run's own tallies and `ledgerDiscrepancy` from the same equation, so it is always 0; `openingBalanceTotal` comes from fixtures, not the issuer. MCN-404-AC2's invariant isn't independently checked. | `gateway-go/internal/chaos/runner.go` `drive` (the ponytail comment says so) | GW + ISS | **Fixed** in #118: opening and closing totals are read from the Issuer Admin API (`GET /v1/cards/{cardRef}` for every seed card, `ISSUER_ADMIN_URL`) at run start and after the SAF drain; `ledgerDiscrepancy = (closing − opening) − (−Σ approved)`; non-zero ⇒ `FAILED` + `LEDGER_MISMATCH`. Other traffic on the seed cards during a run shows up as a mismatch |
| CHA-G2 | The run's final state is broadcast as `chaos.changed` with a `ChaosRun` payload, violating `ws-events.schema.json` (`chaos.changed` → `ChaosScenario`). The page therefore gets the verdict only from the 2 s poll. | `runner.go` `finish` → `BroadcastChaos("chaos.changed", snapshot)` | GW | **Fixed** in #118: every snapshot, final included, is broadcast as `chaos.run.progress` |
| CHA-G3 | Any purchase-service error ends the run `FAILED` with discrepancy 0, and the UI shows "Sổ sách lệch" (a money discrepancy) for what is an infrastructure error. `CONNECTION_CUT` can trigger it. | `runner.go` `drive` (`finish(runID, statusFailed, 0, 0)`); `MoneyVerificationPanel.tsx` | GW + contracts + WEB | **Fixed** in #118: a service or infrastructure error ends the run `FAILED` with `failureKind: RUN_ERROR` and a PAN-masked `failureDetail`; discrepancy stays 0. WEB part done in #124: a `RUN_ERROR` run shows "Lần chạy thử bị lỗi" with its `failureDetail` |
| CHA-G4 | `PUT` doesn't validate `scenarioId`: an unknown id answers 200 with `"id": ""`, broadcasts it, and pollutes in-memory state. A missing `enabled` silently disables. | `internal/api/chaos.go` `handleSetChaosScenario`; `toxiproxy_client.go` `specFor` default | GW | **Fixed** in #118: unknown `scenarioId` ⇒ 404 `not-found`; missing or null `enabled` ⇒ 400 `validation-error` |
| CHA-G5 | `Idempotency-Key` presence is enforced but not deduplicated (docs/04 §2 requires a 24 h replay), so a retried `POST /v1/chaos/runs` starts a second run. `transactions` has no upper bound (contract max 10000). No guard against concurrent runs. | `internal/api/chaos.go` `handleStartChaosRun` | GW | **Fixed** in #118: `Idempotency-Key` must be a UUID; same key and body replays the stored 202 (in memory, not expired), a different body ⇒ 422 `idempotency-key-mismatch`; `transactions` 1–10000 else 400; 409 `conflict` while a run is RUNNING/VERIFYING |
| CHA-G6 | There is no list-runs endpoint and runs live only in memory, so the page can't show the latest run after a reload, navigation or gateway restart (Ruling R2). | `openapi.yaml` has no `GET /v1/chaos/runs`; `runner.go` `runs` map | contracts + GW | **Fixed** in #118 (provider): `GET /v1/chaos/runs?limit=` newest first, `startedAt` set. Runs stay in memory, as the contract says |
| CHA-G7 | A 404 on the run poll (gateway restarted) keeps polling every 2 s indefinitely with retries, and the run button stays disabled, because the last snapshot is RUNNING. | `chaos-client.ts` `useChaosRun` (`refetchInterval` looks only at `data`) | WEB | **Fixed** in #124: a 404 on the run poll stops retries and polling (`ChaosRunGoneError`); the screen treats the run as absent, so the panel resets and the run button is enabled |
| CHA-G8 | `VERIFYING` is in the contract and in the page's copy ("Đang kiểm chứng…") but the provider never sets it. The SAF-drain phase reports `RUNNING`, and `reversed` stays 0 until the end. | `runner.go` (no `statusVerifying`) | GW | **Fixed** in #118: `VERIFYING` is set and broadcast for the SAF drain and verification |
| CHA-G9 | The page copy disagrees with the provider: `SLOW_NETWORK` "± 200" (jitter is 0); `ISSUER_DOWN` "→ STIP", "circuit OPEN · STIP limit 500.000 ₫" (MCN-802 not built; the provider adds the same `reset_peer` as `CONNECTION_CUT`); the API's own `easyText`/`technicalText` go unused (Ruling R1). | `messages/vi.json` `chaos.scenarios.*`; `toxiproxy_client.go` `specFor` | WEB + GW | Align copy with the provider until MCN-802; make `ISSUER_DOWN` distinct (e.g. `timeout` toxic or proxy disable) |
| CHA-G10 | A failing scenarios GET (Toxiproxy down) renders all six cards off with "Mọi thứ bình thường": a false calm state with no error. | `ChaosLabScreen.tsx` (`useChaosScenarios().data ?? []`) | WEB | **Fixed** in #124: a failed scenarios read shows "Không tải được danh sách sự cố. Hãy thử lại."; no cards and no calm pill |
| CHA-G11 | `DROP_RESPONSE` can't be enabled on a default stack: 501 unless `CHAOS_FAKE_ISSUER_ADDR` is set (MCN-407). The card offers it anyway, and the click ends in the generic alert. | `toxiproxy_client.go` `SetScenario`; `gateway-go/compose.yaml` | GW + WEB | **Fixed** in #118: `PUT` answers 501 `scenario-unavailable`, and `GET /v1/chaos/scenarios` reports `available: false` for `DROP_RESPONSE` when `CHAOS_FAKE_ISSUER_ADDR` is unset (contract #116). WEB part done in #124: the card is disabled with a one-line reason |
| CHA-G12 | ~~Problem types are bare slugs (`idempotency-key-required`, `chaos-set-failed`…), not docs/04 URIs, and `idempotency-key-required` differs from the catalogue's `insufficient-idempotency-key`.~~ | `internal/api/chaos.go` | GW | **Fixed**: slugs in #118; the URI form, `instance` and `traceId` in #PRN (P-1) |
| CHA-G13 | `chaos.run.progress` carries no sequence number, so a delayed WS event can briefly regress the panel below a newer polled snapshot. | `runner.go`; `ChaosLabScreen.tsx` `setQueryData` | GW + WEB | **Fixed**: provider in #118 (`seq`), web in #124 (`newerRun` keeps the higher `seq` for WS updates and polls) |
| CHA-G14 | ~~The chaos runner's cardToken → cardRef map (`chaos.fixtureCardRefs`) duplicates `contracts/fixtures/cards.json`, which `internal/purchase/gen` already generates from. A sync test guards it.~~ | `gateway-go/internal/chaos/runner.go`; `TestFixtureCardRefs_matchContractFixtures__CHA_G1` | GW | **Fixed** in #PRN: the generator writes each card's `CardRef` into `purchase.CardFixture` from `contracts/fixtures/cards.json`; the chaos runner reads it from the seed cards, and `fixtureCardRefs` and its sync test are gone |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version, verified against main @ `8d27c72` and the local stack (GET only) |
| 1.1 | 2026-09-25 | CHA-G1–G6, G8, G11, G12 and G13 fixed on the provider side (#118); §4.3/§4.4 provider rules rewritten for the issuer-verified run; CHA-G14 added |
| 1.2 | 2026-09-26 | CHA-G7, CHA-G10, CHA-G13 fixed on the web side; the web also disables `available: false` scenarios (CHA-G11) and shows `RUN_ERROR` as an error (CHA-G3) (#124) |
| 1.3 | 2026-09-26 | CHA-G12 URI half and CHA-G14 fixed (#PRN) |
