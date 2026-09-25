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
| 3.1 | `GET /v1/metrics/overview` | gateway-go | on load, then every 30 s | KPIs, throughput bars, decline reasons | **Yes**: page renders nothing until it succeeds | Implemented; bucketing and decline grouping differ from this spec (§6) |
| 3.2 | `GET /v1/transactions?limit=8` | gateway-go | once on load | Feed table seed rows | No | Implemented |
| 3.3 | `WS /v1/stream` (`transaction.created`, `transaction.updated`) | gateway-go | push | Feed table live rows | No | `created` only; `updated` never sent |
| 3.4 | `GET /v1/network/links` | gateway-go | every 5 s | Header badge, health row 1 | No | Implemented |
| 3.5 | `GET /v1/network/saf` | gateway-go | every 5 s | Health row 2 | No | Implemented |
| 3.6 | `GET /v1/network/switch` | gateway-go | every 5 s | Health row 3 | No | **Missing: returns 404** |
| 3.7 | `GET /v1/keys/acquirer` | gateway-go | once on load | Health row 4 | No | Implemented; empty until the first rotation |

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

Provider rule: the keys the gateway actually uses (`ZPK_HEX`, `ZAK_HEX` at startup) must appear as `ACTIVE` rows. **Today `key_store` is written only by a rotation (G4)**, so a fresh stack returns `[]` and the row is hidden even though the gateway is using a ZPK.

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
| G2 | Throughput is 1-minute, sparse buckets (`date_trunc('minute')`, only minutes with rows, `tps = count/60`) | `internal/store/overview.go` `overviewThroughput` | GW | 24 × 150 s buckets via `generate_series`, zero-filled, `tps = count/150` (rule 3.1-1). Also add the bucket size to the schema description |
| G3 | `transaction.updated` never broadcast | only `BroadcastTransaction("transaction.created", …)` in `purchase`/`advtxn` | GW | Broadcast on every status transition, including SAF reversal completion |
| G4 | `key_store` empty until the first rotation | `INSERT INTO key_store` only in `rotation/runner.go`; migration creates the table empty | GW | On startup, register the configured ZPK/ZAK as `ACTIVE` when no active row exists |
| G5 | Canvas groups the tail into "Lý do khác"; the UI lists every code | `DeclineReasonsBreakdown.tsx` renders all rows | WEB | Keep the provider presentation-free; fold everything after the top 4 into one "Lý do khác" row client-side |
| G6 | `TransactionSummary.latencyMs` is always `null` | `toSummaryDTO` never sets it | GW | Not used by this page; fix with the Journey screen |
| G7 | "Today" is the UTC day, so in Vietnam (UTC+7) the KPIs reset at 07:00 local | `now.Truncate(24h)` in `store.Overview` | GW + product | Decide: business date (cutover) vs local calendar day. Record as a ruling |
| G8 | No loading/error state for the page | `OverviewScreen` returns `null` without data | WEB | Skeletons, plus a problem banner on failure |

## 7. Seed data check

Question asked: after `make up && make seed`, does the backend hold data that makes this page look like the canvas? **No.** What exists today:

| Page element | Canvas / `dev:mock` | Real backend after `make up && make seed` | Match |
| --- | --- | --- | --- |
| Transactions, KPIs, bars, decline reasons | 1 950 today, 94.2 %, 7 merchants, 5 decline codes | Nothing: `gateway-go/Makefile` `seed` only prints `acquirer seed arrives with MCN-303`. Rows exist only after someone uses the POS | ✗ |
| Merchant names | 7 Vietnamese merchants with diacritics | 1 merchant, `Ca phe Goc Pho` (no diacritics), and every purchase is hard-wired to it (`fixedMerchantID`) | ✗ |
| Cards | ••••4417, 9021, 3310, 7765, plus 1208 and 5540 | 4 fixture cards: 4417 normal (5 000 000 ₫), 9021 low balance (80 000 ₫), 3310 **BLOCKED**, 7765 **expired** (08/26) | Partial |
| Issuer link | SIGNED_ON, echo seconds ago | Migration seeds `issuer`/`DISCONNECTED`; becomes SIGNED_ON once the gateway signs on | ✓ |
| SAF queue | empty | empty | ✓ |
| Stand-in / circuit | STIP OFF · CLOSED | endpoint 404 | ✗ (G1) |
| Security key | ZPK `3F9A21`, 26 days left | none until a rotation; lifetime is 365 days in the gateway, not the 90 the mock uses | ✗ (G4) |

Observed on the running stack (2026-09-25): 3 transactions in total, all from 2026-09-24, all card 4417 and merchant `Ca phe Goc Pho`. Two are stuck in `SENT` and one in `REVERSAL_PENDING` (RC 96, the key-rotation gap noted in `docs/plans/MCN-503.md`). `metrics/overview` returns all zeros because none of them are "today".

The mock itself also has rows no real backend could produce from the fixtures: ••••3310 (blocked) shown as approved and as "Sai mã PIN", and ••••9021 (80 000 ₫ balance) with a pending 358 000 ₫ purchase. The canvas cards 1208 and 5540 do not exist in the fixtures at all.

### Recommended seed (MCN-303 follow-up)

To make the real stack reproduce the canvas without bypassing the ledger (root CLAUDE.md §9):

1. **Fixtures (contract PR):** add the canvas merchants to `contracts/fixtures/cards.json` `terminals` (one terminal each, with Vietnamese names and MCCs). Decide whether to add fixture cards for 1208/5540 or map those canvas rows onto the existing four.
2. **Gateway:** resolve the merchant from the terminal instead of `fixedMerchantID`; seed `merchant`/`terminal` from the fixture file.
3. **`make seed` for gateway-go:** drive real `POST /v1/transactions/purchases` calls, which keeps tran_log, the issuer ledger and the WS events consistent. Pick card and amount so each outcome is the real one: 4417 approved, 9021 over 80 000 ₫ → RC 51, 3310 → RC 62, 7765 → RC 54, wrong PIN on 4417 → RC 55. The Chaos API gives a reversal (RC 91 → REVERSED). Backdating rows for the day-over-day delta and the 60-minute bars would need a seed-only path; decide whether that is acceptable.
4. **Mocks:** once 1–3 exist, regenerate `scenario-handlers.ts` rows from the same fixture outcomes so `dev:mock` and the real stack show the same kinds of rows.
