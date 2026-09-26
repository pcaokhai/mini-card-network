# Overview page: API contract

| | |
| --- | --- |
| Document | `docs/api/overview-page.md` |
| Version | 2.0 |
| Status | Approved for integration. One call (`GET /v1/network/switch`) is not built yet: §9 OVW-G1 |
| Date | 2026-09-25 |
| Screen | route `/`, container `web-next/src/app/(console)/overview/OverviewScreen.tsx` (mounted by `src/app/(console)/page.tsx`) |
| Stories | MCN-306 (WEB); MCN-303, MCN-304, MCN-204 links and SAF, MCN-501/504 keys (GW); MCN-802 switch (GW, planned) |
| Provider(s) | gateway-go: `internal/api/overview.go`, `transactions_query.go`, `network.go`, `keys.go`; WS hub `internal/ws/hub.go` |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/overview-client.ts`, `network-client.ts`, `security-client.ts`, and `src/shared/ws/useWsEvents.ts` |
| Design reference | canvas `project/Main.dc.html` (docs/01-prd.md §7.1) |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only, through the BFF on :3000) |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and [`docs/04-api-contract.md`](../04-api-contract.md) §2–3, and are not repeated.

## 1. Purpose and scope

The Overview is the console's landing page. It shows today's KPIs, the last hour's throughput, decline reasons, a live transaction feed, and the health of the issuer link, the SAF queue, stand-in processing and the working key. It is read-only.

This contract covers the six REST reads and the WebSocket feed the page consumes. Out of scope: the actions on the Network and Security screens that change the same resources (sign-on, echo, key rotation). Those screens have their own page contracts.

## 2. Page map

```
┌ Header ─────────────────────────────────────────────────────────────┐
│ search · [issuer link badge ← §4.4]            [Dễ hiểu | Chuyên sâu] │
├ Sidebar ─┬ Title "Tổng quan hôm nay" + date (client clock, §4.8)    │
│ business │ ┌ KPI ─┐┌ KPI ─┐┌ KPI ─┐┌ KPI ─┐   ← §4.1 metrics/overview │
│ date     │ └──────┘└──────┘└──────┘└──────┘                          │
│ (§4.8)   │ ┌ Giao dịch trực tiếp ───────┐ ┌ Sức khỏe hệ thống ─────┐│
│          │ │ throughput bars   ← §4.1   │ │ issuer link  ← §4.4     ││
│          │ │ feed table ← §4.2 + §5 WS  │ │ SAF queue    ← §4.5     ││
│          │ │                            │ │ stand-in     ← §4.6     ││
│          │ │                            │ │ security key ← §4.7     ││
│          │ │                            │ ├ Vì sao … bị từ chối §4.1┤│
│          │ │                            │ ├ Chốt ngày (client §4.8) ┤│
└──────────┴─┴────────────────────────────┴─┴─────────────────────────┘
```

| UI region (label) | Data shown | Call(s) | Refresh |
| --- | --- | --- | --- |
| Header link badge | Issuer link state | §4.4 | 5 s poll |
| Title "Tổng quan hôm nay", date | Browser date | §4.8 | none |
| KPI cards (4) | Transactions today, approval rate, p99/p50 latency, ledger match | §4.1 | 30 s poll |
| "Giao dịch trực tiếp": bars | 24 × 150 s TPS samples | §4.1 | 30 s poll |
| "Giao dịch trực tiếp": feed table | Latest transactions | §4.2 seed, then §5 | once, then push |
| "Sức khỏe hệ thống" rows 1–4 | Link, SAF, stand-in, key | §4.4–§4.7 | 5 s (rows 1–3), once (row 4) |
| "Vì sao giao dịch bị từ chối" | Decline shares by RC | §4.1 | 30 s poll |
| "Chốt ngày" countdown, sidebar business date | Browser clock | §4.8 | 1 s tick |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/metrics/overview` | gateway-go | KPIs, bars, decline reasons | on mount, then every 30 s | n/a | none | Real |
| 4.2 | `GET /v1/transactions?limit=8` | gateway-go | Feed seed rows | once on mount | n/a | none | Real |
| 4.3 | `WS /v1/stream` | gateway-go | Feed live rows (§5) | push | n/a | none | Real, with OVW-G9 |
| 4.4 | `GET /v1/network/links` | gateway-go | Header badge, health row 1 | every 5 s | n/a | none | Real |
| 4.5 | `GET /v1/network/saf` | gateway-go | Health row 2 | every 5 s | n/a | none | Real |
| 4.6 | `GET /v1/network/switch` | gateway-go | Health row 3 | every 5 s | n/a | none | dev:mock only; Planned MCN-802 (404 today) |
| 4.7 | `GET /v1/keys/acquirer` | gateway-go | Health row 4 | once on mount | n/a | none | Real |
| 4.8 | client-only | browser clock | Title date, business date, cutover countdown | 1 s tick | n/a | n/a | Client only |

Every REST call is a `GET`, idempotent, and cached per query key by TanStack Query. A failing call hides only the element it feeds, except 4.1 (§8).

## 4. Calls

### 4.1 GET /v1/metrics/overview

- **Summary.** Today's KPIs, the last 60 minutes of throughput and the decline breakdown, in one snapshot.
- **Request.** No parameters, no body. Headers: only `traceparent`, which the BFF forwards when present.
- **Response.** `200`, schema `Overview`. "Today" is the acquirer's current business date (ADR-007), returned as `businessDate`. It rolls at `CUTOVER_TIME` (default 23:59:59) in `CUTOVER_TZ` (default Asia/Ho_Chi_Minh), so the KPIs reset once, at cutover. Until MCN-702 persists real cutovers, the gateway derives it from the clock: a late or manual cutover isn't reflected.

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `transactionsToday` | integer | ✓ | KPI 1 value (counts up) | `1.950` (vi-VN grouping) | same |
| `transactionsDeltaPct` | number, fraction | – | KPI 1 sub-line | `Tăng 12% so với hôm qua` / `Giảm …`; hidden when absent | not shown |
| `throughput[]` | `{at, tps}[]` | ✓ | Bars; KPI 1 Expert sub-line | bars only | `TPS đỉnh {max tps} lúc {HH:mm of that sample}` |
| `approvalRate` | number 0–1 | ✓ | KPI 2 value | `94,2%` (1 decimal max) + `Ổn định trong 1 giờ qua` | `RC 00 / tổng 0100 + 0200` |
| `p99LatencyMs` | integer | ✓ | KPI 3 value | `212 ms` + `Nhanh, dưới ngưỡng 300 ms` (or `Chậm, vượt …` at ≥ 300) | same value |
| `p50LatencyMs` | integer | – | KPI 3 Expert sub-line | not shown | `p99 end-to-end · p50 96 ms`; `p99 end-to-end` alone when absent |
| `ledgerMatches` | boolean | ✓ | KPI 4 value + sub-line | `Khớp 100%` + `Không có chênh lệch nào` / mismatch copy | `Σ ledger = Σ tran_log · lệch 0 ₫` / `Σ ledger ≠ Σ tran_log · cần đối soát` |
| `declineReasons[]` | `{responseCode, label, share}[]` | ✓ | "Vì sao giao dịch bị từ chối" bars, sorted by `share` desc | localised label (§4.9) | `label · RC {responseCode}`; code omitted when `responseCode` is `""` |

- **Provider rules.**
  1. `throughput` is 24 samples of 150 s (2.5 min) covering the last 60 minutes, ascending by `at`, and dense: a bucket with no transactions is present with `tps: 0`. `tps` = transactions in the bucket / 150. The UI draws one bar per sample, scaled to the largest `tps`, and highlights the last one as "now". The Expert caption says "TPS theo từng 2,5 phút · 60 phút qua", so a different bucket size makes the caption wrong.
  2. `approvalRate` = approved / (approved + declined) today; `0` when neither exists.
  3. `p50LatencyMs` / `p99LatencyMs` are percentiles of SENT → first terminal state (APPROVED, DECLINED, TIMED_OUT) today, in ms; `0` when there is no sample.
  4. `transactionsDeltaPct` compares the business date with the previous one over the same time elapsed since each opened (its cutover instant), as a fraction (`0.12` = +12 %). It is omitted when yesterday's window has no transactions; `0` never means "unknown".
  5. `declineReasons[].share` values sum to 1. The provider returns every code, presentation-free. A declined row always carries an RC: a response without DE 39 is recorded as RC `30` (format error), and older rows with no RC are left out of `declineReasons`.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 500 | `overview-read-failed` | Database read failed | Whole page body stays blank (OVW-G8) |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | BFF can't reach the gateway | same |

- **Example.** Real response from the local stack (2026-09-25, trimmed). The canvas numbers are reproduced by `pnpm dev:mock` (`web-next/src/mocks/scenario-handlers.ts`).

```json
{
  "transactionsToday": 153,
  "transactionsDeltaPct": 0.31896551724137934,
  "approvalRate": 0.8896551724137931,
  "p50LatencyMs": 3,
  "p99LatencyMs": 25,
  "ledgerMatches": true,
  "throughput": [
    { "at": "2026-09-25T08:32:57.351351628Z", "tps": 0 },
    "… 22 more, 150 s apart …",
    { "at": "2026-09-25T09:30:27.351351628Z", "tps": 0 }
  ],
  "declineReasons": [
    { "responseCode": "62", "label": "Card is blocked", "share": 0.3125 },
    { "responseCode": "51", "label": "Insufficient funds", "share": 0.25 },
    { "responseCode": "61", "label": "Limit exceeded", "share": 0.1875 },
    { "responseCode": "54", "label": "Card is expired", "share": 0.125 },
    { "responseCode": "", "label": "Declined", "share": 0.0625 },
    { "responseCode": "30", "label": "Message format error", "share": 0.0625 }
  ]
}
```

- **Notes.** No ETag and no caching headers. Client: `useOverview()` with `refetchInterval` 30 000 ms; otherwise TanStack defaults (3 retries with backoff, refetch on window focus). Rate limit: not enforced.

### 4.2 GET /v1/transactions?limit=8

- **Summary.** The eight newest transactions. The page uses them only as the feed's starting rows until live events arrive (§5), and doesn't paginate.
- **Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `limit` | query | integer | no | contract 1–200; this page sends `8` | 50 |

- **Response.** `200`, `{ items: TransactionSummary[], nextCursor }`. The page reads `items` only.

| Field | Type | Req. | UI element (column) | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `createdAt` | date-time | ✓ | Giờ | `HH:mm:ss` local | same |
| `merchantName` | string | ✓ | Cửa hàng | truncated with ellipsis | same |
| `maskedPan` | string | ✓ | Thẻ | `•••• 4417` | same |
| `amount` | Money | ✓ | Số tiền | `250.000 ₫` | same |
| `rrn` | string | ✓ | RRN column; row link to `/transactions/{rrn}` | screen-reader only | `626514000123` (header "RRN (DE 37)") |
| `status` + `responseCode` | enum + string | ✓ / – | Kết quả badge (§4.9) | `Không đủ tiền` | `Không đủ tiền · RC 51` |
| `responseLabel` | string | – | fallback label only (§4.9) | – | – |
| `stan`, `type`, `terminalId`, `latencyMs` | | | not shown on this page | | |

- **Provider rules.** Newest first (`createdAt` desc, then id desc); `limit` honoured; `maskedPan` is first 6 + last 4; `latencyMs` is 0200 sent → 0210 received, `null` when the issuer never answered.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `validation-error` | `limit` not an integer from 1 to 200, or another bad parameter | Feed shows `Chưa có giao dịch` until a WS event arrives |
| 500 | `transactions-read-failed` | Database read failed | same |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | BFF can't reach the gateway | same |

- **Example.** Real item from the local stack:

```json
{
  "rrn": "626807000352", "stan": "000352", "type": "PURCHASE",
  "status": "APPROVED", "responseCode": "00", "responseLabel": "Approved",
  "amount": { "amount": 10000, "currency": "704" },
  "maskedPan": "970436******4417", "terminalId": "00000042",
  "merchantName": "Cà phê Góc Phố", "latencyMs": 8059,
  "createdAt": "2026-09-25T07:57:55.625583Z"
}
```

- **Notes.** Client: `useRecentTransactions()`, query key `["transactions","recent"]`, no interval. Rate limit: not enforced.

### 4.3 WS /v1/stream

The consumed events and their rules are in §5.

### 4.4 GET /v1/network/links

- **Summary.** The acquirer's ISO links. The gateway has one, to the issuer.
- **Request.** No parameters.
- **Response.** `200`, `Link[]`. The UI picks the link whose `to` equals `issuer` (case-insensitive), falling back to the first.

| Field / condition | Header badge | Health row "Kết nối tới ngân hàng phát hành" (Easy) | (Expert) |
| --- | --- | --- | --- |
| `status` ∈ {`SIGNED_ON`,`CONNECTED`} | green `Đã kết nối ngân hàng phát hành` | green, `Ổn định · kiểm tra {s} giây trước` | `{status} · echo 0800/301 {s}s trước` |
| `status` ∈ {`DOWN`,`DISCONNECTED`} | red `Mất kết nối ngân hàng phát hành` | red, `Mất kết nối, đang thử kết nối lại` | same pattern |
| no response yet / empty array | amber `Đang kiểm tra kết nối` | row hidden | row hidden |
| `lastEchoAt` | – | `{s}` = seconds since, ticking every second | same |
| `lastEchoOk`, `p99LatencyMs`, `inFlight` | not shown on this page | | |

- **Provider rules.** `lastEchoAt` updates on every echo (0800/301), so the "N giây trước" age stays small on a healthy link. The gateway returns exactly one element, `linkId: "issuer"`.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 500 | `link-read-failed` | Link state read failed | Badge amber, row hidden |

- **Example.** Real:

```json
[{ "linkId": "issuer", "from": "gateway", "to": "issuer", "status": "SIGNED_ON",
   "lastEchoAt": "2026-09-25T09:31:59.286755Z", "lastEchoOk": true, "p99LatencyMs": null, "inFlight": 0 }]
```

- **Notes.** Client: `useLinks()`, every 5 s. The cache is shared with the Network screen.

### 4.5 GET /v1/network/saf

- **Summary.** Store-and-forward queue depth. This page reads only `depth` and `deadCount`.
- **Response.** `200`, `{ depth, deadCount, items[] }`.

| Condition | Tone | Easy | Expert |
| --- | --- | --- | --- |
| `depth = 0` | green | `Trống, không có lệnh nào chờ` | `SAF depth 0 · 0 dead` |
| `depth > 0`, `deadCount = 0` | amber | `{depth} lệnh đang chờ gửi lại` | `SAF depth {depth} · {dead} dead` |
| `deadCount > 0` | red | same as above | same |

- **Errors.** 500 `saf-read-failed`: the row is hidden.
- **Example.** Real: `{"depth":0,"deadCount":0,"items":[]}`.
- **Notes.** Client: `useSafQueue()`, every 5 s.

### 4.6 GET /v1/network/switch

- **Summary.** Circuit-breaker and stand-in (STIP) state.
- **Response.** `200`, `SwitchStatus`. **Not implemented in gateway-go: it returns a plain-text `404 page not found` today (OVW-G1)**, so this row never appears against the real backend.

| Condition | Tone | Easy | Expert |
| --- | --- | --- | --- |
| `circuit = CLOSED`, `stipActive = false` | green | `Đang tắt vì hệ thống chính hoạt động tốt` | `STIP OFF · circuit CLOSED` |
| `stipActive = true` | amber | `Đang bật, tự duyệt giao dịch nhỏ thay ngân hàng` | `STIP ON · circuit {circuit}` |
| `circuit = OPEN` | red | per `stipActive` | per `stipActive` |

  `stipLimit` and `stipApprovedCount` are not shown on this page.
- **Example.** dev:mock (`scenario-handlers.ts`): `{"circuit":"CLOSED","stipActive":false,"stipLimit":{"amount":500000,"currency":"704"},"stipApprovedCount":0}`.
- **Notes.** Client: `useSwitchStatus()`, every 5 s. Against the real stack each poll is a 404, which TanStack retries 3 times.

### 4.7 GET /v1/keys/acquirer

- **Summary.** The acquirer's working keys, KCV only. The UI reads the `ZPK` with `status = ACTIVE` and hides the row if there is none.

| Condition | Tone | Easy | Expert |
| --- | --- | --- | --- |
| `daysRemaining > 7` | green | `Còn hiệu lực, đổi khóa sau {days} ngày` | `ZPK KCV {kcv} · rotate T-{days}d` |
| `1 ≤ daysRemaining ≤ 7` | amber | same | same |
| `daysRemaining ≤ 0` | red | `Đã hết hạn, cần đổi khóa ngay` | same |

- **Provider rules.** The keys the gateway actually uses appear as `ACTIVE` rows. At startup the gateway registers `ZAK_HEX` / `ZPK_HEX` (the values the issuer is configured with) as `ACTIVE` when `key_store` has none of that type; a key set by a rotation is never overwritten. Only the KCV is returned, never key material.
- **Errors.** 500 `list-keys-failed`: the row is hidden.
- **Example.** Real:

```json
[{ "keyType": "ZAK", "counterparty": null, "kcv": "D927EE", "status": "ACTIVE", "activatedAt": "2026-09-25T06:29:06.530923Z", "daysRemaining": 365, "lifetimeDays": 365 },
 { "keyType": "ZPK", "counterparty": null, "kcv": "3B84E8", "status": "ACTIVE", "activatedAt": "2026-09-25T06:29:06.53306Z", "daysRemaining": 365, "lifetimeDays": 365 }]
```

- **Notes.** Client: `useAcquirerKeys()` (`security-client.ts`), fetched once and invalidated when a rotation completes on the Security screen.

### 4.8 Client-only data (no API)

| Element | Source | Note |
| --- | --- | --- |
| Title date `Thứ Hai, 21/09/2026` | browser clock | |
| Sidebar "Ngày giao dịch" | browser clock | Should become the settlement business date (`/v1/settlement/days/…`) once R7 ships |
| "Chốt ngày tiếp theo · Còn X giờ Y phút" | browser clock to 23:59:59 local (docs/03 §7.6) | Same R7 dependency |

### 4.9 Display rules the provider should know

The UI localises result labels itself, because the gateway's `responseLabel` and `declineReasons[].label` are English only:
- `APPROVED` / `DECLINED`: the label comes from the RC table (docs/03 §8, `messages/*.json` → `responseCodes`); for an RC not in the table, the server label.
- Other statuses are labelled by status: `CREATED`/`SENT` → `Đang chờ`, `TIMED_OUT` → `Quá thời gian`, `REVERSAL_PENDING` → `Đang tự hủy`, `REVERSED` → `Đã tự hủy`, `FAILED` → `Thất bại`.

A provider that adds an RC must add it to docs/03 §8 and to both message files; otherwise it shows in English.

| Status | Badge tone |
| --- | --- |
| APPROVED | green |
| DECLINED, FAILED | red |
| REVERSED | purple |
| CREATED, SENT, TIMED_OUT, REVERSAL_PENDING | amber |

## 5. Real-time events

Envelope `{ id, type, occurredAt, data }` per `contracts/ws-events.schema.json`; `data` is a `TransactionSummary`. Consumer: `LiveFeed`, through `useWsEvents(["transaction.created","transaction.updated"])`. Other event types are ignored.

| Event | Payload | UI effect |
| --- | --- | --- |
| `transaction.created` | `TransactionSummary` | New RRN: the row is inserted on top with a slide-in highlight |
| `transaction.updated` | `TransactionSummary` | Known RRN: the row is replaced in place (for example SENT → APPROVED, REVERSAL_PENDING → REVERSED) |

Ordering and dedupe (UI side; the provider may rely on these):
- Rows merge by `rrn`: a later event for a shown RRN replaces that row. The table keeps at most 30 rows.
- Above 5 events/s the UI batches renders every 500 ms. The UI drops no event.
- On close, the socket reconnects with exponential backoff from 1 s up to 30 s. Events missed while disconnected are not replayed.

Provider rules:
1. Send `transaction.created` when a transaction reaches its first visible state, and `transaction.updated` on every later status change, including SAF-driven reversal completion. The gateway sends `transaction.created` once, after the outcome, and `transaction.updated` on a cancellation (REVERSAL_PENDING), a SAF acknowledgement (REVERSED) and a late response.
2. `data` carries every required `TransactionSummary` field, and `responseCode` is set for APPROVED/DECLINED.
3. The hub drops a message for a client whose 16-message buffer is full (`internal/ws/hub.go`), so a slow browser can miss rows.

## 6. Security and compliance

- PAN appears only as `maskedPan` (first 6 + last 4), and the feed renders the last four only. No call on this page returns a PIN, track data or key material.
- Keys: KCV only (§4.7).
- The page is read-only: there is no audit trail to write and no destructive action.
- The WebSocket accepts any origin and takes no token (`CheckOrigin` returns true). docs/04 §1 specifies a 60-second token from `POST /api/stream-token` (OVW-G10).

## 7. Non-functional requirements

| Call | Budget | Observed on the local stack (2026-09-25) | Load from this page |
| --- | --- | --- | --- |
| 4.1 metrics | no NFR for the call | ~5 ms through the BFF | 2 req/min per open tab |
| 4.2 transactions | no NFR | p50 5 ms, max 11 ms (30 samples) | 1 per mount |
| 4.4 / 4.5 / 4.6 | no NFR | ~5 ms | 12 req/min each per tab (4.6: plus 3 retries per 404) |
| 4.7 keys | no NFR | ~5 ms | 1 per mount |
| p99 shown on KPI 3 | NFR-01: end-to-end p99 < 300 ms | 25 ms | – |

Payload bounds: `throughput` always has 24 elements; `declineReasons` has at most one element per distinct RC (≤ 22 from docs/03 §8, plus `""`); the feed request is capped at 8 items.

## 8. UI states

| Situation | What renders | Driving call |
| --- | --- | --- |
| `metrics/overview` loading or failed | **The whole page body is blank** (no skeleton, no error) | 4.1 (OVW-G8) |
| `metrics/overview` empty (new day) | KPIs `0`, `0%`, `0 ms`; bars at zero; the decline card shows its title only | 4.1 |
| `transactions` empty and no WS event | Feed shows `Chưa có giao dịch` | 4.2, §5 |
| links / saf / switch / keys failed or empty | That health row is hidden; the others still render. A failed links call also leaves the header badge amber | 4.4–4.7 |
| WS disconnected | The feed stops updating silently; REST polling continues | §5 |
| Provider not available (switch) | The stand-in row never appears | 4.6 |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| OVW-G1 | `GET /v1/network/switch` not implemented | `curl :3000/api/v1/network/switch` → 404; no route in `gateway-go/internal/api` | GW | MCN-802 AC1, after MCN-801 (`feat/MCN-801-switch`, unmerged) |
| OVW-G2 | ~~Throughput was 1-minute, sparse buckets over 30 minutes~~ | **Fixed**: 24 dense × 150 s buckets, `tps = count/150` (verified live) | GW | Done (MCN-002 seed work). Documenting the bucket size in the schema description is still open |
| OVW-G3 | ~~`transaction.updated` is never broadcast~~ | **Fixed** (#117): `transaction.updated` on cancellation, late response and SAF acknowledgement (`reversalAckAnnouncer` → `purchase.Service.BroadcastUpdate`) | GW | Done |
| OVW-G4 | ~~`key_store` empty until the first rotation, so a fresh stack had no ZAK and every purchase failed its MAC~~ | **Fixed**: startup registers `ZAK_HEX`/`ZPK_HEX` (verified live, §4.7) | GW | Done (MCN-002 seed work) |
| OVW-G5 | The canvas folds the tail into "Lý do khác"; the UI lists every code | `DeclineReasonsBreakdown.tsx` sorts and renders all rows | WEB | **Fixed** in #125: the top four coded reasons, the rest folded client-side into "Lý do khác" |
| OVW-G6 | ~~`TransactionSummary.latencyMs` always `null`~~ | **Fixed**: `toSummaryDTO` sets it from `journey.LatencyMs` (live values `8059`, `20`) | GW | Done (MCN-304 journey canvas) |
| OVW-G7 | ~~"Today" is the UTC day, so in Vietnam (UTC+7) the KPIs reset at 07:00 local~~ | **Fixed** (#120): ADR-007. `bizdate.Calendar` (clock adapter on `CUTOVER_TIME`/`CUTOVER_TZ`) sets `tran_log.business_date` and DE 15 at send; the Overview counts `business_date = Current`, compares with `Previous` over the same time since each cutover, and returns `businessDate`; `Transaction.businessDate` is the stored column | GW + product | Done |
| OVW-G8 | No loading or error state for the page | `OverviewScreen.tsx`: `if (!overviewQuery.data) return null;` | WEB | **Fixed** in #125: a loading skeleton and a problem banner |
| OVW-G9 | The live feed's socket never connects on the BFF origin | `useWsEvents` falls back to `ws://{location.host}/v1/stream`; `curl :3000/v1/stream` → 404; `NEXT_PUBLIC_WS_URL` is set nowhere in the repo | WEB + PLAT | **Fixed** in #124 (documented): `web-next/.env.example` sets `NEXT_PUBLIC_WS_URL`; Route Handlers can't proxy the upgrade |
| OVW-G10 | WS authentication differs from docs/04 §1 | The hub's `CheckOrigin` accepts every origin and reads no `token`; there is no `/api/stream-token` route under `web-next/src/app/api` | GW + WEB | Implement the token, or amend docs/04 through an ADR |
| OVW-G11 | ~~A declined row without an RC is counted under `responseCode: ""`~~ | **Fixed** (#117): `purchase.ResponseCodeOf` records a response without DE 39 as RC `30`; `overviewDeclineReasons` skips rows with no RC | GW | Done |
| OVW-G12 | ~~Gateway problem responses don't follow docs/04 §3~~ | `problem()` in `internal/api/lab.go` writes the bare slug as both `type` and `title`, with no `https://mcn.local/problems/` prefix, no `instance`, no `traceId`, and the raw Go error as `detail` | GW | **Fixed** (#PRN, P-1): one problem writer (`internal/api/problems.go`) with the docs/04 §3 shape; `internal` 500s carry a generic detail and the `traceId`, the cause goes to the log |

### 9.1 Seed data check

Question asked: after `make up && make seed`, does the backend hold data that makes this page look like the canvas? **Yes, since the MCN-002 seed work**, with the exceptions below. Before it, the gateway seed only printed a placeholder, every purchase used one hard-wired merchant, and a fresh stack could not complete a purchase or a reversal at all.

`make seed` (`gateway-go/cmd/seed`, plan [`docs/plans/MCN-002-acquirer-seed.md`](../plans/MCN-002-acquirer-seed.md)) upserts the fixture merchants and terminals, signs the issuer link on, sets ••••1208's per-transaction limit through the issuer Card Admin API, and sends real `POST /v1/transactions/purchases` calls, so `tran_log`, the issuer ledger and the WS events stay consistent. It cancels two approvals, which sends real 0420s, and waits for REVERSED. It then backdates the gateway's rows so that the last hour follows the canvas's bar shape and yesterday's window gives the day-over-day delta.

Verified on a fresh stack (2026-09-25, `make down -v && make up && make seed`, first try):

| Page element | Canvas | Real backend after `make seed` | Match |
| --- | --- | --- | --- |
| Transactions today / delta | 1 950, +12 % | 130 today, +12 % vs the same window yesterday (116). Volume is scaled down: the gateway is driven one purchase at a time | Shape ✓, volume scaled |
| Approval rate | 94.2 % | 93.8 % (231 of 246 approved across both days) | ✓ |
| 60-minute bars | 24 bars, canvas heights | 24 non-empty 150 s buckets in the canvas's proportions | ✓ |
| Decline reasons | 51, 61, 55, 62, 54 + "Lý do khác" | 51, 61, 62, 54 from real issuer decisions. No RC 55 (R-12) | Partial (R-12, OVW-G5) |
| Merchants / cards | 7 merchants, ••••4417/9021/3310/7765/1208/5540 | The same 7 merchants and 6 cards, all from `contracts/fixtures/cards.json` | ✓ |
| "Đã tự hủy" rows | 1 | 2 cancellations reach REVERSED on both hosts, with the issuer's reversing journal | ✓ |
| Issuer link, SAF, key | SIGNED_ON, empty, ZPK KCV | SIGNED_ON, SAF depth 0 / 0 dead, ZPK registered from `ZPK_HEX` | ✓ |
| Stand-in / circuit | STIP OFF · CLOSED | endpoint 404 | ✗ (OVW-G1) |

A second run on the same stack adds another 246 rows. Every RRN stays unique across a gateway restart.

Defects the seed exposed (all fixed):

| Defect | Effect | Fix |
| --- | --- | --- |
| The issuer never MACed its 0210 | Every real purchase declined RC 96 | ISS `Respond` signs responses |
| RCs 06, 10, 17, 62, 68, 95 missing from the issuer's RC table | An RC 62 decline failed with an FK violation | ISS migration V6 |
| 0420 chain: DE 90 had a blank STAN, the advice lacked mandatory fields and a MAC, send errors were swallowed, the issuer answered every 0420 with RC 30, and the gateway ACKed any 0430 | No reversal had ever completed end to end (root CLAUDE.md §6.4) | GW `saf` advice and worker, ISS listener routing. [`docs/plans/MCN-401-reversal-0420-fix.md`](../plans/MCN-401-reversal-0420-fix.md) |
| An aborted reversal was answered "00", and two reversals of one purchase could both post | The acquirer could mark REVERSED money the issuer never returned, or the issuer could return it twice | ISS: only "00" once recorded; guarded status update in the journal transaction |
| The STAN counter restarted at 1 on every reconnect and restart | 492 rows, 250 distinct RRNs | GW shared counter resumed from `tran_log` |
| Sign-on waited on a dead socket with no timeout | The first seed on a fresh stack hit `409 broken pipe` until the gateway was restarted | GW supervisor: connection-scoped sign-on with a timeout |
| Bars scaled to `max(1, peak)` and TPS rendered as a raw float | A flat chart at real (< 1 TPS) volume | WEB |
| Mocks embedded full test PANs | PCI rule (root CLAUDE.md §6.2) broken in web source | WEB: last-4 only |

Still open for this page: OVW-G1, G10, G12. Also:
- **R-12**: the gateway never forwards a PIN block, so RC 55 ("Sai mã PIN") can't be produced (`docs/09-risk-register.md`).
- A REVERSED row shows its original approval's RC 00 ("Đã tự hủy · RC 00"). Showing the 0420's reason code (for example 17, customer cancellation) needs a new `TransactionSummary` field. That is a contract change, so it goes in its own contract PR.

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First integration contract and seed-data check (#82) |
| 2.0 | 2026-09-25 | Rewritten into the per-page template, with real examples from the local stack. G2, G4 and G6 marked fixed. Added G9–G12 (WS origin, WS authentication, empty-RC decline bucket, problem format) |
| 2.1 | 2026-09-26 | OVW-G5, OVW-G8 fixed; OVW-G9 closed by #124; the date shown is `Overview.businessDate` (ADR-007) (#125) |
| 2.2 | 2026-09-25 | OVW-G3 and OVW-G11 fixed (#117) |
| 2.3 | 2026-09-26 | OVW-G7 fixed (#120): "today" is the acquirer's business date (ADR-007) |
| 2.4 | 2026-09-26 | OVW-G12 fixed (#PRN, P-1) |
