# Overview page — backend integration contract

Version 1.0 · 2026-09-25 · Story MCN-306 · Screen `/` (`web-next/src/app/(console)/overview/OverviewScreen.tsx`)

Normative sources, in precedence order: `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, then this document. This page adds what the schema cannot express: which UI element reads each field, cadence, ordering and bucketing rules, and how the UI behaves on empty or failed responses. Conventions (errors, money, time, pagination) are in [`docs/04-api-contract.md`](../04-api-contract.md) §2–3 and are not repeated.

Design reference: canvas `project/Main.dc.html` (docs/01-prd.md §7.1). Every example payload below is the `pnpm dev:mock` scenario (`web-next/src/mocks/scenario-handlers.ts`), which reproduces the canvas numbers.

## 1. Page map

```
┌ Header ─────────────────────────────────────────────────────────────┐
│ search · [issuer link badge ← §3.4]            [Dễ hiểu | Chuyên sâu] │
├ Sidebar ─┬ Title "Tổng quan hôm nay" + date (client clock, §3.8)    │
│ business │ ┌ KPI ─┐┌ KPI ─┐┌ KPI ─┐┌ KPI ─┐   ← §3.1 metrics/overview │
│ date     │ └──────┘└──────┘└──────┘└──────┘                          │
│ (§3.8)   │ ┌ Giao dịch trực tiếp ───────┐ ┌ Sức khỏe hệ thống ─────┐│
│          │ │ throughput bars   ← §3.1   │ │ issuer link  ← §3.4     ││
│          │ │ feed table ← §3.2 + §3.3 WS│ │ SAF queue    ← §3.5     ││
│          │ │                            │ │ stand-in     ← §3.6     ││
│          │ │                            │ │ security key ← §3.7     ││
│          │ │                            │ ├ Vì sao bị từ chối ← §3.1┤│
│          │ │                            │ ├ Chốt ngày (client §3.8) ┤│
└──────────┴─┴────────────────────────────┴─┴─────────────────────────┘
```

## 2. Call inventory

| # | Call | Provider | Cadence | Feeds | Gates page render | Gateway status (2026-09-25) |
| --- | --- | --- | --- | --- | --- | --- |
| 3.1 | `GET /v1/metrics/overview` | gateway-go | on load, then every 30 s | KPIs, throughput bars, decline reasons | **Yes**: page renders nothing until it succeeds | Implemented; decline grouping is a UI gap (§6 G5) |
| 3.2 | `GET /v1/transactions?limit=8` | gateway-go | once on load | Feed table seed rows | No | Implemented |
| 3.3 | `WS /v1/stream` (`transaction.created`, `transaction.updated`) | gateway-go | push | Feed table live rows | No | `created` only; `updated` never sent |
| 3.4 | `GET /v1/network/links` | gateway-go | every 5 s | Header badge, health row 1 | No | Implemented |
| 3.5 | `GET /v1/network/saf` | gateway-go | every 5 s | Health row 2 | No | Implemented |
| 3.6 | `GET /v1/network/switch` | gateway-go | every 5 s | Health row 3 | No | **Missing: returns 404** |
| 3.7 | `GET /v1/keys/acquirer` | gateway-go | once on load | Health row 4 | No | Implemented; the configured ZAK/ZPK are registered at startup |

All REST calls are `GET`, idempotent, need no headers beyond the conventions in docs/04 §2, and are cached per query key by TanStack Query. A failing call hides only the element it feeds, except 3.1 (see §5).

## 3. Calls

### 3.1 `GET /v1/metrics/overview`

Schema `Overview`. Covers today, where "today" is the UTC calendar day of the request.

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `transactionsToday` | integer | ✓ | KPI 1 value (counts up) | `1.950` (vi-VN grouping) | same |
| `transactionsDeltaPct` | number, fraction | – | KPI 1 sub-line | `Tăng 12% so với hôm qua` / `Giảm …`; hidden when absent | not shown |
| `throughput` | array | ✓ | Bars; KPI 1 Expert sub-line | – | `TPS đỉnh {max tps} lúc {HH:mm of that sample}` |
| `approvalRate` | number 0–1 | ✓ | KPI 2 value | `94,2%` (1 decimal max) + `Ổn định trong 1 giờ qua` | `RC 00 / tổng 0100 + 0200` |
| `p99LatencyMs` | integer | ✓ | KPI 3 value | `212 ms` + `Nhanh, dưới ngưỡng 300 ms` (or `Chậm, vượt …` at ≥ 300) | – |
| `p50LatencyMs` | integer | – | KPI 3 Expert sub-line | – | `p99 end-to-end · p50 96 ms`; `p99 end-to-end` alone when absent |
| `ledgerMatches` | boolean | ✓ | KPI 4 value + sub-line | `Khớp 100%` + `Không có chênh lệch nào` / mismatch copy | `Σ ledger = Σ tran_log · lệch 0 ₫` / `Σ ledger ≠ Σ tran_log · cần đối soát` |
| `declineReasons[]` | array | ✓ | "Vì sao giao dịch bị từ chối" bars, sorted by `share` desc | label | `label · RC {responseCode}`; code omitted when `responseCode` is `""` |

Provider rules (what the UI assumes):

1. **`throughput`: 24 samples of 150 s (2.5 min) covering the last 60 minutes, ascending by `at`, dense (a bucket with no transactions is present with `tps: 0`).** `tps` is transactions per second in that bucket (`count / 150`). The UI draws one bar per sample, scaled to the largest `tps`, and highlights the last one as "now". The Expert caption says "TPS theo từng 2,5 phút · 60 phút qua", so a different bucket size makes that caption wrong.
2. `approvalRate` = approved / (approved + declined) today; 0 when neither exists.
3. `p50LatencyMs` / `p99LatencyMs` = percentiles of SENT → first terminal state (APPROVED, DECLINED, TIMED_OUT) today, in ms, `0` when there is no sample.
4. `transactionsDeltaPct` compares today with **the same elapsed window yesterday**, as a fraction (`0.12` = +12 %). Omit it when yesterday's window has no transactions; never send `0` to mean "unknown".
5. `declineReasons[].share` values sum to 1 across the array. `label` is used only as a fallback: the UI labels known RCs itself (§4.1). Grouping into "Lý do khác" is a UI decision (§6, G5); the provider returns every code.

Example:

```json
{
  "transactionsToday": 1950,
  "transactionsDeltaPct": 0.12,
  "approvalRate": 0.942,
  "p99LatencyMs": 212,
  "p50LatencyMs": 96,
  "ledgerMatches": true,
  "throughput": [
    { "at": "2026-09-21T06:32:30Z", "tps": 12 },
    "… 22 more, 150 s apart …",
    { "at": "2026-09-21T07:30:00Z", "tps": 26 }
  ],
  "declineReasons": [
    { "responseCode": "51", "label": "Insufficient funds", "share": 0.41 },
    { "responseCode": "55", "label": "Incorrect PIN", "share": 0.23 },
    { "responseCode": "61", "label": "Limit exceeded", "share": 0.18 },
    { "responseCode": "62", "label": "Card is blocked", "share": 0.12 },
    { "responseCode": "", "label": "Other", "share": 0.06 }
  ]
}
```

### 3.2 `GET /v1/transactions?limit=8`

Schema `{ items: TransactionSummary[], nextCursor }`. The UI only reads `items`, and only as the table's starting rows until live events arrive (§3.3). It does not paginate.

Provider rules: newest first (`createdAt` desc); `limit` honoured; `maskedPan` is first 6 + last 4 (the UI shows `•••• {last4}`).

| Field | UI column | Easy | Expert |
| --- | --- | --- | --- |
| `createdAt` | Giờ | `HH:mm:ss` local | same |
| `merchantName` | Cửa hàng | truncated with ellipsis | same |
| `maskedPan` | Thẻ | `•••• 4417` | same |
| `amount` | Số tiền | `250.000 ₫` (vi-VN currency) | same |
| `rrn` | RRN column; row link to `/transactions/{rrn}` | screen-reader only | `626514000123` (column header is the glossary term "RRN (DE 37)") |
| `status` + `responseCode` | Kết quả badge (label and tone, §4) | `Không đủ tiền` | `Không đủ tiền · RC 51` |
| `responseLabel` | – | fallback only (§4.1) | – |
| `stan`, `type`, `terminalId`, `latencyMs` | not shown on this page | | |

Example item:

```json
{
  "rrn": "626514000122", "stan": "000122", "type": "PURCHASE",
  "status": "DECLINED", "responseCode": "51", "responseLabel": "Insufficient funds",
  "amount": { "amount": 1240000, "currency": "704" },
  "maskedPan": "970436******9021", "terminalId": "00000042",
  "merchantName": "Nhà sách Ánh Dương", "latencyMs": 107,
  "createdAt": "2026-09-21T07:31:51Z"
}
```

### 3.3 `WS /v1/stream`: `transaction.created`, `transaction.updated`

Envelope `{ id, type, occurredAt, data }` per `contracts/ws-events.schema.json`; `data` is a `TransactionSummary`. Other event types on the stream are ignored by this page.

UI behaviour, which the provider can rely on:

- Rows merge by `rrn`: a later event for an RRN already shown **replaces** that row (so `transaction.updated` moves a row from `SENT` to `APPROVED`, or from `REVERSAL_PENDING` to `REVERSED`). New RRNs go to the top and slide in with a highlight. The table keeps at most 30 rows.
- Above 5 events/s the UI batches renders every 500 ms; no event is dropped.

Provider rules:

1. Send `transaction.created` when the transaction reaches its first visible state, and `transaction.updated` on every later status change (including SAF-driven reversal completion). **Today the gateway sends only `transaction.created`, once, after the outcome** (G3), so a reversal that completes later never updates on screen.
2. `data` must carry every required `TransactionSummary` field; `responseCode` must be set for APPROVED/DECLINED.

### 3.4 `GET /v1/network/links`

Schema `Link[]`. The UI picks the link whose `to` equals `issuer` (case-insensitive), falling back to the first link.

| Field | Header badge | Health row "Kết nối tới ngân hàng phát hành" (Easy) | (Expert) |
| --- | --- | --- | --- |
| `status` ∈ {`SIGNED_ON`,`CONNECTED`} | green `Đã kết nối ngân hàng phát hành` | green, `Ổn định · kiểm tra {s} giây trước` | `{status} · echo 0800/301 {s}s trước` |
| `status` ∈ {`DOWN`,`DISCONNECTED`} | red `Mất kết nối ngân hàng phát hành` | red, `Mất kết nối, đang thử kết nối lại` | same pattern |
| no response yet / empty array | amber `Đang kiểm tra kết nối` | row hidden | row hidden |
| `lastEchoAt` | – | `{s}` = seconds since, ticking every second | same |

Provider rule: `lastEchoAt` updates on every echo (0800/301) so the "N giây trước" age stays small on a healthy link.

### 3.5 `GET /v1/network/saf`

Schema `{ depth, deadCount, items[] }`. Only `depth` and `deadCount` are read.

| Condition | Tone | Easy | Expert |
| --- | --- | --- | --- |
| `depth = 0` | green | `Trống, không có lệnh nào chờ` | `SAF depth 0 · 0 dead` |
| `depth > 0`, `deadCount = 0` | amber | `{depth} lệnh đang chờ gửi lại` | `SAF depth {depth} · {dead} dead` |
| `deadCount > 0` | red | same as above | same |

### 3.6 `GET /v1/network/switch`

Schema `SwitchStatus`. **Not implemented in gateway-go: returns 404 today (G1)**, so this health row never appears against the real backend. It is MCN-802 AC1 (circuit breaker and STIP, slice S8), which depends on MCN-801's switch (`feat/MCN-801-switch`, unmerged).

| Condition | Tone | Easy | Expert |
| --- | --- | --- | --- |
| `circuit = CLOSED`, `stipActive = false` | green | `Đang tắt vì hệ thống chính hoạt động tốt` | `STIP OFF · circuit CLOSED` |
| `stipActive = true` | amber | `Đang bật, tự duyệt giao dịch nhỏ thay ngân hàng` | `STIP ON · circuit {circuit}` |
| `circuit = OPEN` | red | per `stipActive` | per `stipActive` |

`stipLimit` and `stipApprovedCount` are not shown on this page (Network screen).

### 3.7 `GET /v1/keys/acquirer`

Schema `KeyInfo[]`. The UI reads the `ZPK` with `status = ACTIVE`; the row is hidden if there is none.

| Condition | Tone | Easy | Expert |
| --- | --- | --- | --- |
| `daysRemaining > 7` | green | `Còn hiệu lực, đổi khóa sau {days} ngày` | `ZPK KCV {kcv} · rotate T-{days}d` |
| `1 ≤ daysRemaining ≤ 7` | amber | same | same |
| `daysRemaining ≤ 0` | red | `Đã hết hạn, cần đổi khóa ngay` | same |

Provider rule: the keys the gateway actually uses must appear as `ACTIVE` rows. At startup the gateway registers `ZAK_HEX` / `ZPK_HEX` (the same values the issuer is configured with) as `ACTIVE` when `key_store` has none of that type; a key set by a rotation is never overwritten.

### 3.8 Client-only data (no API)

| Element | Source | Note |
| --- | --- | --- |
| Title date `Thứ Hai, 21/09/2026` | browser clock | |
| Sidebar "Ngày giao dịch" | browser clock | Should become the business date from settlement (`/v1/settlement/days/…`) once R7 ships |
| "Chốt ngày tiếp theo · Còn X giờ Y phút" | browser clock to 23:59:59 local (docs/03 §7.6) | Same R7 dependency |

## 4. Display rules the backend should know

### 4.1 Result labels

The UI localises labels itself, because the gateway's `responseLabel` / `declineReasons[].label` are English-only:

- `APPROVED` / `DECLINED`: label from the RC table (docs/03 §8, `messages/*.json` → `responseCodes`); for an RC not in the table, the server label.
- Every other status is labelled by status: `CREATED`/`SENT` → `Đang chờ`, `TIMED_OUT` → `Quá thời gian`, `REVERSAL_PENDING` → `Đang tự hủy`, `REVERSED` → `Đã tự hủy`, `FAILED` → `Thất bại`.

A backend that adds a new RC must add it to docs/03 §8 and both message files; otherwise it shows in English.

### 4.2 Badge tones

| Status | Tone |
| --- | --- |
| APPROVED | green |
| DECLINED, FAILED | red |
| REVERSED | purple |
| CREATED, SENT, TIMED_OUT, REVERSAL_PENDING | amber |

## 5. Loading, empty and error behaviour

| Situation | UI today |
| --- | --- |
| `metrics/overview` loading or failed | **Whole page body is blank** (no skeleton, no error). See G8 |
| `metrics/overview` empty (new day) | KPIs `0`, `0%`, `0 ms`; bars absent; decline card shows its title only |
| `transactions` empty and no WS event | Feed shows `Chưa có giao dịch` |
| links / saf / switch / keys failed or empty | That health row is hidden; the others still render |
| WS disconnected | Feed stops updating silently; the page keeps polling the REST calls |

## 6. Gaps against this contract

| ID | Gap | Evidence | Owner | Proposed fix |
| --- | --- | --- | --- | --- |
| G1 | `GET /v1/network/switch` not implemented | `curl :8080/v1/network/switch` → 404; no route in `internal/api` | GW | MCN-802 AC1 (after MCN-801) |
| G2 | ~~Throughput was 1-minute, sparse buckets over 30 minutes~~ | fixed: 24 dense × 150 s buckets, `tps = count/150` | GW | Done (MCN-002 seed work). Adding the bucket size to the schema description is still open |
| G3 | `transaction.updated` never broadcast | only `BroadcastTransaction("transaction.created", …)` in `purchase`/`advtxn` | GW | Broadcast on every status transition, including SAF reversal completion. (The REST side is fixed: an acknowledged 0420 now moves `tran_log` from `REVERSAL_PENDING` to `REVERSED`; before, reversals stayed pending forever.) |
| G4 | ~~`key_store` empty until the first rotation, so a fresh stack had no ZAK and every purchase failed its MAC~~ | fixed: startup registers `ZAK_HEX`/`ZPK_HEX` | GW | Done (MCN-002 seed work) |
| G5 | Canvas groups the tail into "Lý do khác"; the UI lists every code | `DeclineReasonsBreakdown.tsx` renders all rows | WEB | Keep the provider presentation-free; fold everything after the top 4 into one "Lý do khác" row client-side |
| G6 | `TransactionSummary.latencyMs` is always `null` | `toSummaryDTO` never sets it | GW | Not used by this page; fix with the Journey screen |
| G7 | "Today" is the UTC day, so in Vietnam (UTC+7) the KPIs reset at 07:00 local | `now.Truncate(24h)` in `store.Overview` | GW + product | Decide: business date (cutover) vs local calendar day. Record as a ruling |
| G8 | No loading/error state for the page | `OverviewScreen` returns `null` without data | WEB | Skeletons, plus a problem banner on failure |

## 7. Seed data check

Question asked: after `make up && make seed`, does the backend hold data that makes this page look like the canvas? **Yes, since the MCN-002 seed work**, with the exceptions below. Before it, the gateway seed only printed a placeholder, every purchase used one hard-wired merchant, and a fresh stack could not complete a purchase or a reversal at all (see "Defects the seed exposed").

`make seed` (`gateway-go/cmd/seed`, plan [`docs/plans/MCN-002-acquirer-seed.md`](../plans/MCN-002-acquirer-seed.md)) upserts the fixture merchants and terminals, signs the issuer link on, sets ••••1208's per-transaction limit through the issuer Card Admin API, and then sends real `POST /v1/transactions/purchases` calls, so `tran_log`, the issuer ledger and the WS events stay consistent. It cancels two approvals, which sends real 0420s, and waits for REVERSED. It then backdates the gateway's rows so the last hour follows the canvas's bar shape and yesterday's window gives the day-over-day delta.

Verified on a fresh stack (2026-09-25, `make down -v && make up && make seed`, first try):

| Page element | Canvas | Real backend after `make seed` | Match |
| --- | --- | --- | --- |
| Transactions today / delta | 1 950, +12 % | 130 today, +12 % vs the same window yesterday (116). Volume is scaled down: the gateway is driven one purchase at a time | Shape ✓, volume scaled |
| Approval rate | 94.2 % | 93.8 % (231 of 246 approved across both days) | ✓ |
| 60-minute bars | 24 bars, canvas heights | 24 non-empty 150 s buckets in the canvas's proportions | ✓ |
| Decline reasons | 51, 61, 55, 62, 54 + "Lý do khác" | 51, 61, 62, 54 from real issuer decisions. No RC 55 (R-12) | Partial (R-12, G5) |
| Merchants / cards | 7 merchants, ••••4417/9021/3310/7765/1208/5540 | Same 7 merchants and 6 cards, all from `contracts/fixtures/cards.json` | ✓ |
| "Đã tự hủy" rows | 1 | 2 cancellations reach REVERSED on both hosts, with the issuer's reversing journal | ✓ |
| Issuer link, SAF, key | SIGNED_ON, empty, ZPK KCV | SIGNED_ON, SAF depth 0 / 0 dead, ZPK registered from `ZPK_HEX` | ✓ |
| Stand-in / circuit | STIP OFF · CLOSED | endpoint 404 | ✗ (G1) |

A second run on the same stack adds another 246 rows. Every RRN stays unique across a gateway restart.

### Defects the seed exposed (fixed)

| Defect | Effect | Fix |
| --- | --- | --- |
| The issuer never MACed its 0210 | Every real purchase declined RC 96 | ISS `Respond` signs responses |
| RCs 06, 10, 17, 62, 68, 95 missing from the issuer's RC table | An RC 62 decline failed with an FK violation | ISS migration V6 |
| 0420 chain: DE 90 had a blank STAN, the advice lacked mandatory fields and a MAC, send errors were swallowed, the issuer answered every 0420 with RC 30, and the gateway ACKed any 0430 | No reversal had ever completed end to end (root CLAUDE.md §6.4) | GW `saf` advice / worker, ISS listener routing. [`docs/plans/MCN-401-reversal-0420-fix.md`](../plans/MCN-401-reversal-0420-fix.md) |
| An aborted reversal was answered "00", and two reversals of one purchase could both post | The acquirer could mark REVERSED money the issuer never returned, or the issuer could return it twice | ISS: only "00" once recorded; guarded status update in the journal transaction |
| The STAN counter restarted at 1 on every reconnect and restart | 492 rows, 250 distinct RRNs | GW shared counter resumed from `tran_log` |
| Sign-on waited on a dead socket with no timeout | The first seed on a fresh stack hit `409 broken pipe` until the gateway was restarted | GW supervisor: connection-scoped sign-on with a timeout |
| Bars scaled to `max(1, peak)` and TPS rendered as a raw float | A flat chart at real (< 1 TPS) volume | WEB |
| Mocks embedded full test PANs | PCI rule (root CLAUDE.md §6.2) broken in web source | WEB: last-4 only |

### Still open

- **G1** `/v1/network/switch` (MCN-802).
- **G3** `transaction.updated` is never broadcast, so a reversal only shows after the next poll.
- **G5** decline tail grouping.
- **G7** UTC "today".
- **R-12** the gateway never forwards a PIN block, so RC 55 ("Sai mã PIN") cannot be produced. `docs/09-risk-register.md`.
- A REVERSED row shows its original approval's RC 00 ("Đã tự hủy · RC 00"). Showing the 0420's reason code (for example 17, customer cancellation) needs a field in `TransactionSummary`. That is a contract change, so it goes in its own contract PR.
