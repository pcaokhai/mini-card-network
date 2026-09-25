# Transaction journey page: API contract

| | |
| --- | --- |
| Document | `docs/api/journey-page.md` |
| Version | 1.0 |
| Status | Approved for integration for purchases. Journeys of pre-authorizations, completions, refunds and balance inquiries show purchase MTIs and money signs today (§9 JRN-G2) |
| Date | 2026-09-25 |
| Screen | route `/transactions` → `web-next/src/app/(console)/transactions/JourneyIndexScreen.tsx`; route `/transactions/{rrn}` → `web-next/src/app/(console)/transactions/[rrn]/JourneyScreen.tsx`. Both render `components/journey/JourneyView.tsx` |
| Stories | MCN-307, MCN-406 (WEB); MCN-304 (GW); contract change #92 (`JourneyStep.code`, enum `StepCode`); MCN-308 (ISS, ledger reads) |
| Provider(s) | gateway-go: `internal/api/transactions_query.go` → `internal/journey` (`builder.go`, `steps.go`, `message.go`, `easytext.go`), reversal lookup `internal/saf`. issuer-jpos Admin API: `adapter/http/CardAdminController.java` |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/journey-client.ts` (`useLatestTransaction`, `useJourney`, `useCustomerLedger`) |
| Design reference | canvas `project/Journey.dc.html` (docs/01-prd.md §7.1); plans [`MCN-307-journey-canvas.md`](../plans/MCN-307-journey-canvas.md), [`MCN-304-journey-canvas.md`](../plans/MCN-304-journey-canvas.md); release notes [R5.2](../releases/R5.2.md) |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only, through the BFF on :3000) |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and [`docs/04-api-contract.md`](../04-api-contract.md) §2–3, and are not repeated.

## 1. Purpose and scope

The journey page replays one transaction step by step: what the POS asked, which ISO 8583 messages crossed between the banks (masked), when each happened, and how the cardholder's money moved. `/transactions` opens the newest transaction of the chosen outcome tab; `/transactions/{rrn}` opens a specific one (from the Overview feed, the POS result or search, MCN-307-AC3).

This contract covers the transaction list lookup, the journey read, the `StepCode` contract (§4.3) and the three issuer ledger reads behind the money panel. Out of scope: `GET /v1/transactions/{rrn}` (not called by this page; the journey embeds the same `Transaction`), and the Message Lab that "Mở trong phòng lab" links to.

## 2. Page map

| UI region (label) | Data shown | Call(s) | Refresh |
| --- | --- | --- | --- |
| Breadcrumb "Tổng quan / Hành trình giao dịch", title | static | – | – |
| Tabs "Chọn giao dịch": "Thành công", "Bị từ chối", "Đã tự hủy" (`?view=approved\|declined\|reversed`) | Newest APPROVED / DECLINED / REVERSED transaction | §4.1 | on tab change (`/transactions` only) |
| Summary header ("Tóm tắt giao dịch") | Headline, merchant, card last four, RRN, total time, message count, "Tiền của khách" | §4.2 | once per RRN |
| "Từng bước, theo thời gian" timeline, "Bước {current}/{total}", playback controls | Steps: actor, title, line, offset | §4.2, §4.3 | once per RRN |
| "Chi tiết bước" | Easy explanation, Expert technical text, message fields ("Nội dung message {mti}") | §4.2, §4.3 | once per RRN |
| "Tiền trong tài khoản khách" | "Trước giao dịch", "Trừ tiền mua hàng", "Được hoàn lại", "Số dư cuối cùng" | §4.4–§4.6, else §4.2 `money` | once per RRN |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/transactions?status={s}&limit=1` | gateway-go | Newest transaction of the tab's outcome | `/transactions` mount and tab change | n/a | none | Real |
| 4.2 | `GET /v1/transactions/{rrn}/journey` | gateway-go | Transaction, steps, money deltas | once per RRN | n/a | none | Real (purchases); other types use purchase MTIs (JRN-G2) |
| 4.3 | `StepCode` contract | – | Step semantics inside 4.2 | – | – | – | Real (#92) |
| 4.4 | `GET /v1/cards` | issuer-jpos | Find the card by `maskedPan` | after 4.2, once per RRN | n/a | none | Real |
| 4.5 | `GET /v1/cards/{cardRef}` | issuer-jpos | Current `ledgerBalance` | after 4.4 | n/a | `ETag` returned, unused | Real |
| 4.6 | `GET /v1/cards/{cardRef}/ledger?limit=200&cursor=…` | issuer-jpos | Journals back to this RRN | after 4.5; up to 10 pages | n/a | none | Real |
| – | `WS /v1/stream` | – | not consumed | – | – | – | – |

4.4–4.6 run inside one query (`useCustomerLedger`, key `["transactions", rrn, "customer-ledger"]`), enabled once the journey's `maskedPan` is known.

## 4. Calls

### 4.1 GET /v1/transactions?status={s}&limit=1

- **Summary.** Returns the newest transaction with the tab's status; the page then loads its journey (§4.2).
- **Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `status` | query | `TransactionStatus` | no (the page always sends it) | `APPROVED` for "Thành công", `DECLINED` for "Bị từ chối", `REVERSED` for "Đã tự hủy" | none |
| `limit` | query | integer | no | 1–200; the page sends `1` | 50 |
| `cursor`, `rc`, `last4`, `from`, `to` | query | | no | not sent by this page | |

- **Response.** `200`, `{ items: TransactionSummary[], nextCursor }`. The page reads `items[0].rrn` only; an empty `items` is the empty state.
- **Provider rules.**
  1. Newest first by (`createdAt`, id) descending; keyset cursor, stable under concurrent inserts.
  2. `status` filters on `tran_log.state` exactly; unknown values are not rejected (JRN-G6).
  3. `limit` ≤ 0 means 50. Values above 200 are not clamped (JRN-G6).
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `invalid-request` | `limit` not an integer, `from`/`to` not RFC 3339 | "Không tải được hành trình. Thử tải lại trang." |
| 500 | `transactions-read-failed` | Database error, or a malformed `cursor` (JRN-G6) | same |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | Gateway unreachable | same |

- **Example.** Real: `GET /api/v1/transactions?status=REVERSED&limit=1`

```json
{
  "items": [{
    "rrn": "626807000345", "stan": "000345", "type": "PURCHASE", "status": "REVERSED",
    "responseCode": "96", "responseLabel": "System error",
    "amount": { "amount": 10000, "currency": "704" }, "maskedPan": "970436******4417",
    "terminalId": "00000042", "merchantName": "Cà phê Góc Phố", "latencyMs": 5,
    "createdAt": "2026-09-25T07:47:16.445986Z"
  }],
  "nextCursor": "MTc5MDMyMjQzNjQ0NTk4NjAwMHwyNjc"
}
```

- **Notes.** Key `["transactions","latest",status]`. TanStack defaults: 3 retries, refetch on window focus (so a newer transaction replaces the shown one when the tab regains focus). Rate limit: not enforced.

### 4.2 GET /v1/transactions/{rrn}/journey

- **Summary.** The transaction plus its ordered steps and money deltas, rebuilt from `tran_log`, `tran_state_history` and the stored 0420 advice in `saf_queue`.
- **Request.**

| Parameter | In | Type | Required | Constraints |
| --- | --- | --- | --- | --- |
| `rrn` | path | string | ✓ | `^[0-9A-Z]{12}$` (not enforced, JRN-G6) |

- **Response.** `200`, schema `Journey`.

`transaction` (schema `Transaction`):

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `type` | enum | ✓ | Headline `{type}` | "Mua hàng", "Tiền ủy quyền trước", "Hoàn tất", "Hoàn tiền", "Tra cứu số dư", "Hủy giao dịch" | same |
| `amount` | Money | ✓ | Headline, step copy, money panel currency | `250.000 ₫` | same |
| `status` | enum | ✓ | Outcome (with `steps`, rule 8): headline, icon, "Tiền của khách", money note | e.g. "{type} {amount} đã được duyệt" / "Giao dịch {amount} đã được tự động hủy" | same |
| `merchantName` | string | ✓ | Summary meta; POS_REQUEST copy | "Cà phê Góc Phố · Thẻ •••• 4417 · Mã tra soát {rrn}" | "… · RRN {rrn}" |
| `maskedPan` | string | ✓ | Last four in meta and step copy; card match for §4.4 | `•••• 4417` | same |
| `rrn` | string | ✓ | Summary meta | "Mã tra soát {rrn}" | "RRN {rrn}" |
| `responseCode` | string\|null | – | ISSUER_DECLINED reason | "Lý do: {reason}." with `reason` from `messages/vi.json` `responseCodes[rc]` | same (the technical line carries `RC {rc}`) |
| `responseLabel` | string\|null | – | Reason fallback when the RC isn't in the message file | English label | same |
| `stan`, `terminalId`, `latencyMs`, `createdAt`, `authCode`, `approvedAmount`, `balance`, `businessDate`, `originalRrn`, `traceId` | | | not shown | | |

`steps[]` (schema `JourneyStep`):

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `seq` | integer | ✓ | Step identity; `money[].atStep` target | – | – |
| `code` | `StepCode` | – (always set by this provider) | Chooses the copy (§4.3) | `journey.step.{code}.title` / `.easy` | title from `code`; line = `technicalText` |
| `actor` | `Actor` | ✓ | Actor label | "Máy POS", "Ngân hàng thanh toán", "Ngân hàng phát hành", "Hàng đợi gửi lại" | "POS terminal", "Acquirer gateway (Go)", "Issuer host (jPOS)", "SAF queue" |
| `offsetMs` | integer ≥ 0 | ✓ | Timeline offset; total time = last step's offset | `+182 ms`, `+30,09 s`; "Tổng thời gian" | "Tổng thời gian (end-to-end)" |
| `kind` | `StepKind` | ✓ | Dot tone: OK green, BAD red, WARN amber, REVERSAL purple, INFO neutral | same | same |
| `technicalText` | string | ✓ | Timeline line and tech box | detail panel only | timeline line and tech box |
| `title`, `easyText` | string | ✓ | Fallback copy when `code` is absent | English | English |
| `message` | `IsoMessage`\|null | – | "Nội dung message {mti}" table; counted in "Số lần hai ngân hàng trao đổi" | field names from `journey.field.{de}` (e.g. "Mã tra soát"), else `easyName` | "Message {mti} · {count} phần tử"; `technicalName`, `F{de}` |
| `message.fields[].value` | string | ✓ | Field value (monospace) | same | same |

`money[]`:

| Field | Type | Req. | UI element | Notes |
| --- | --- | --- | --- | --- |
| `label` | string | ✓ | not shown (the UI labels rows itself) | English: `Purchase`, `Refund` |
| `delta` | int64, signed | ✓ | "Trừ tiền mua hàng" (negative) / "Được hoàn lại" (positive) | Used only when the issuer ledger isn't available (rule 10) |
| `balanceAfter` | int64\|null | – | not used | Always `null` (JRN-G4) |
| `atStep` | integer | ✓ | Row is revealed when playback reaches this `seq` | |

- **Provider rules.**
  1. `steps[].seq` runs 1..n without gaps, in display order.
  2. `offsetMs` is measured from the earliest of `created_at`, `sent_at` and the first `tran_state_history` row, and never decreases from one step to the next (clamped, because history rows, `saf_queue` and second-precision DE 7 come from separate sources). A LATE_RESPONSE step is inserted at its own offset.
  3. The step sequence per outcome is fixed (§4.3 table). A transaction still in SENT ends after REQUEST_SENT.
  4. `message` is present exactly on REQUEST_SENT, ISSUER_APPROVED, ISSUER_DECLINED, REVERSAL_SENT and REVERSAL_CONFIRMED (the last two only when a stored advice exists). The first field is `{"de":"MTI"}`; the rest are in DE order and only fields with a stored value appear.
  5. Messages are rebuilt from stored columns, not captured bytes (plan MCN-304 Rulings 4–5): the 0210 and 0430 echo the request's fields; DE 38 appears only on RC `00`/`10`; the 0430's DE 39 is always `00` (only a `00` ACKs an advice); the 0420/0421 fields are the stored advice, so DE 90 is exactly what the SAF worker sent. `IsoField.raw` is never returned (JRN-G8).
  6. DE 2 is the stored masked PAN passed through `obs.MaskPAN` again. DE 52, DE 64 and DE 128 are never returned.
  7. REVERSAL_SENT carries MTI `0421` when the advice took more than one send (`saf_queue.attempts` counts failed sends), else `0420`.
  8. The client derives the outcome from `transaction.status` and the steps: REVERSED with a NO_RESPONSE step ⇒ auto-reversed ("đã được tự động hủy"), REVERSED without one ⇒ cancelled ("đã được hủy"); TIMED_OUT or REVERSAL_PENDING ⇒ reversing; CREATED/SENT ⇒ pending.
  9. Provider `money`: the debit sits at ISSUER_APPROVED; for an unknown outcome at NO_RESPONSE; for a declined response that still queued a reversal (MAC failure) at ISSUER_DECLINED. A plain decline has no rows. The credit sits at REVERSAL_CONFIRMED. `delta` is `-amount` for the debit and `+amount` for the credit, whatever the type (JRN-G2).
  10. The web prefers the issuer ledger (§4.4–§4.6, plan MCN-307 Ruling 3): it rewinds the card's current `ledgerBalance` through its journals to just before this RRN's first journal, then shows "Trước giao dịch", the RRN's own journals and "Số dư cuối cùng". It falls back to rule 9's deltas when the card or the RRN's journals aren't found, and shows nothing for a DECLINED transaction.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | `unknown-transaction` | No `tran_log` row for `{rrn}` (including malformed RRNs) | "Không tìm thấy giao dịch {rrn}." after 3 retries |
| 500 | `transaction-read-failed` | `tran_log` read failed | same message (JRN-G9) |
| 500 | `journey-read-failed` | State history or reversal lookup failed | same |
| 502 | `…/upstream-unavailable` (BFF) | Gateway unreachable | same |

- **Example.** Real, `GET /api/v1/transactions/626807000352/journey` (approved purchase, trimmed):

```json
{
  "transaction": {
    "rrn": "626807000352", "stan": "000352", "type": "PURCHASE", "status": "APPROVED",
    "responseCode": "00", "responseLabel": "Approved",
    "amount": { "amount": 10000, "currency": "704" }, "maskedPan": "970436******4417",
    "terminalId": "00000042", "merchantName": "Cà phê Góc Phố", "latencyMs": 8059,
    "createdAt": "2026-09-25T07:57:55.625583Z", "authCode": "L7AFDG",
    "approvedAmount": null, "balance": null, "businessDate": "2026-09-25",
    "originalRrn": null, "traceId": "626807000352"
  },
  "steps": [
    { "seq": 1, "code": "POS_REQUEST", "actor": "POS", "offsetMs": 0, "kind": "OK",
      "title": "POS sends the request", "easyText": "The card was read and the terminal asked for payment.",
      "technicalText": "POST /v1/purchases · TID 00000042 · entry mode 052", "message": null },
    { "seq": 2, "code": "REQUEST_SENT", "actor": "ACQUIRER", "offsetMs": 1, "kind": "OK",
      "title": "Request sent to the issuer", "easyText": "The acquirer packed the request and sent it to the card's bank.",
      "technicalText": "0200 · STAN 000352 · RRN 626807000352 · amount 10000 704",
      "message": { "mti": "0200", "fields": [
        { "de": "MTI", "easyName": "Message type", "technicalName": "Message type indicator", "format": "n 4", "value": "0200" },
        { "de": "2", "easyName": "Card number (masked)", "technicalName": "PAN", "format": "n..19 LLVAR", "value": "970436******4417" },
        "… DE 3, 4, 7, 11, 22, 37, 41, 42, 49 …"
      ] } },
    { "seq": 3, "code": "ISSUER_APPROVED", "actor": "ISSUER", "offsetMs": 8059, "kind": "OK",
      "technicalText": "0210 · RC 00 · DE 38 L7AFDG · 8058 ms after the 0200", "message": { "mti": "0210", "fields": ["…"] }, "…": "…" },
    { "seq": 4, "code": "POS_RESULT", "actor": "POS", "offsetMs": 8059, "kind": "OK",
      "technicalText": "status APPROVED · RC 00", "message": null, "…": "…" }
  ],
  "money": [ { "label": "Purchase", "delta": -10000, "balanceAfter": null, "atStep": 3 } ]
}
```

  Real POS cancellation (`626807000296`, REVERSED), steps only: POS_REQUEST +0 → REQUEST_SENT +1 (0200) → ISSUER_APPROVED +32 (0210, DE 38 `K78MJW`) → POS_RESULT +32 → REVERSAL_QUEUED +142 ("reason 17") → REVERSAL_SENT +143 (0420, DE 39 `17`, DE 90 `020000029609250717270000097049900000000000`) → REVERSAL_CONFIRMED +636 (0430, DE 39 `00`). `money`: `Purchase −250000 @3`, `Refund +250000 @7`.
- **Notes.** Key `["transactions", rrn, "journey"]`; no refetch interval (JRN-G5). TanStack defaults: 3 retries (≈ 7 s before the error shows), refetch on window focus. No ETag. Rate limit: not enforced.

### 4.3 StepCode contract (#92)

`JourneyStep.code` says what a step is, independent of language. Clients render their own copy from it; `title`/`easyText`/`technicalText` stay as the provider's English fallback (`contracts/openapi.yaml` `StepCode`). The web renders every code below from `messages/*.json` → `journey.step`.

| Code | Actor | Kind | Message | Emitted when | Easy title (vi) |
| --- | --- | --- | --- | --- | --- |
| `POS_REQUEST` | POS | OK | none | Always, first | "Máy POS gửi yêu cầu" |
| `REQUEST_SENT` | ACQUIRER | OK | 0200 | The transaction reached SENT (history) or has `sent_at` | "Đóng gói và gửi sang ngân hàng phát hành" |
| `LOCAL_DECLINE` | ACQUIRER | BAD | none | Never sent (link not signed on, RC 91) | "Chưa kết nối được ngân hàng phát hành" |
| `ISSUER_APPROVED` | ISSUER | OK | 0210 | Reached APPROVED | "Ngân hàng phát hành đồng ý" |
| `ISSUER_DECLINED` | ISSUER | BAD | 0210 | Reached DECLINED after sending | "Ngân hàng phát hành từ chối" |
| `NO_RESPONSE` | ACQUIRER | WARN | none | Reached TIMED_OUT | "Hết thời gian chờ" (plus the countdown ring during autoplay, MCN-406-AC2) |
| `REVERSAL_QUEUED` | SAF | REVERSAL | none | Reached REVERSAL_PENDING | "Lưu lệnh hủy trước khi gửi" |
| `POS_RESULT` | POS | OK or BAD | none | Always after the outcome; carries the status the POS was answered with | OK: "Máy POS in hóa đơn"; BAD after NO_RESPONSE or LOCAL_DECLINE: "Máy POS báo không thành công"; other BAD: "Máy POS báo bị từ chối" |
| `REVERSAL_SENT` | ACQUIRER | REVERSAL | 0420, or 0421 after a failed send | The stored advice has a DE 7 (it was sent), or REVERSED without an advice | 0420: "Gửi lệnh hủy"; 0421: "Gửi lại lệnh hủy" |
| `REVERSAL_CONFIRMED` | ISSUER | REVERSAL | 0430 | The advice was ACKed, or REVERSED | "Ngân hàng phát hành hoàn tiền" |
| `LATE_RESPONSE` | ISSUER | WARN | none | A 0210 arrived after the timeout (`late_response_at`) | "Câu trả lời cũ đến muộn" |

Sequences per outcome (R5.2):

| Outcome | Steps |
| --- | --- |
| approved | POS_REQUEST → REQUEST_SENT → ISSUER_APPROVED → POS_RESULT |
| declined | POS_REQUEST → REQUEST_SENT → ISSUER_DECLINED → POS_RESULT (BAD) |
| link-down | POS_REQUEST → LOCAL_DECLINE → POS_RESULT (BAD) |
| timeout | POS_REQUEST → REQUEST_SENT → NO_RESPONSE → REVERSAL_QUEUED → POS_RESULT (BAD) → REVERSAL_SENT → REVERSAL_CONFIRMED, with LATE_RESPONSE inserted by offset when one arrived |
| POS cancellation | approved sequence → REVERSAL_QUEUED (reason 17) → REVERSAL_SENT → REVERSAL_CONFIRMED |
| MAC failure | declined sequence (RC 96) → REVERSAL_QUEUED (reason 06) → REVERSAL_SENT → REVERSAL_CONFIRMED |

Rules for providers and clients:
1. Adding a value to `StepCode` is additive for the schema but not for clients: a client must fall back to `title`/`easyText` for a code it doesn't know. The web falls back only when `code` is absent; an unknown code renders a missing-message key (JRN-G11). Until that is fixed, a new code ships with its copy in both message files in the same release.
2. The provider never emits kind `INFO` or actors `NETWORK`/`SWITCH` today; the web styles them anyway.
3. A code's meaning never changes; a new behaviour gets a new code.

### 4.4 GET /v1/cards

- **Summary.** All issuer cards (summaries). The page finds the one whose `maskedPan` equals the journey's.
- **Response.** `200`, `CardSummary[]`: `cardRef`, `maskedPan`, `holderName`, `status`, `expiry`. Only `cardRef` and `maskedPan` are read.
- **Errors.** None besides 502 from the BFF; a failure makes the money panel fall back to the provider deltas.
- **Example.** Real: `[{"cardRef":"crd_normal0001","maskedPan":"970436******4417","holderName":"Nguyen Minh Anh","status":"ACTIVE","expiry":"11/28"}, "… 5 more …"]`.

### 4.5 GET /v1/cards/{cardRef}

- **Summary.** The matched card's detail; only `ledgerBalance.amount` is read. Same endpoint and example as [pos-page.md](pos-page.md) §4.6.
- **Errors.** 404 `https://mcn.local/problems/not-found` (unknown `cardRef`): fallback to provider deltas.

### 4.6 GET /v1/cards/{cardRef}/ledger

- **Summary.** The account's journals, newest first, paged until the page passes this RRN's journals.
- **Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `cardRef` | path | string | ✓ | `^crd_[A-Za-z0-9]{10,32}$` | |
| `limit` | query | integer | no | 1–200; the page sends `200` | 50 |
| `cursor` | query | string | no | the previous page's `nextCursor` (a journal id) | |

- **Response.** `200`, `{ items: JournalEntry[], nextCursor }`. Read: `rrn`, `postings[].account` (customer postings start with `ACC-`), `postings[].direction`, `postings[].amount.amount`.
- **Provider rules.** Newest first by journal id; `nextCursor` is the last item's journal id when the page is full, else `null`. Every entry balances (debits = credits).
- **Client rules.** Stop when there is no `nextCursor`, or when this RRN was seen and the last item of the page belongs to another RRN (a transaction's journals are adjacent). At most 10 pages (2 000 journals); beyond that the balances can't be rewound and the panel falls back to deltas.
- **Errors.** 404 `https://mcn.local/problems/not-found` for an unknown card; a non-numeric `limit` or `cursor` is an unhandled `500 Server Error` (plain text). Either one falls back to provider deltas.
- **Example.** Real, `?limit=2`:

```json
{
  "items": [
    { "journalId": "244", "occurredAt": "2026-09-25T07:58:03.676262Z", "description": "PURCHASE journal 244",
      "entryType": "PURCHASE", "rrn": "626807000352",
      "postings": [
        { "account": "ACC-crd_normal0001", "direction": "DEBIT", "amount": { "amount": 10000, "currency": "704" } },
        { "account": "SETTLEMENT_SUSPENSE", "direction": "CREDIT", "amount": { "amount": 10000, "currency": "704" } }
      ] },
    "… journal 243 …"
  ],
  "nextCursor": "243"
}
```

## 5. Real-time events

None. A journey loaded while the transaction is still SENT, TIMED_OUT or REVERSAL_PENDING doesn't update until a reload or a window refocus (JRN-G5; the gateway doesn't send `transaction.updated` either, OVW-G3).

## 6. Security and compliance

- The only card identifier returned is `maskedPan` (first 6 + last 4), including DE 2 inside every rebuilt message, which is re-masked by `obs.MaskPAN` even if a clear PAN were ever stored (plan MCN-304 Ruling 3).
- PIN block (DE 52) and MACs (DE 64/128) are never returned; neither is stored.
- The ledger match uses `maskedPan`, never a PAN; the ledger's customer account is `ACC-{cardRef}`, not a PAN.
- `traceId` is the RRN, not a real trace id (JRN-G3); NFR-08 correlation still works by RRN.
- The page is read-only: no audit, no destructive action.

## 7. Non-functional requirements

| Call | Budget | Observed on the local stack (2026-09-25, 30 samples via the BFF) | Load from this page |
| --- | --- | --- | --- |
| 4.1 list | no NFR | p50 5 ms, max 11 ms | 1 per tab change |
| 4.2 journey | no NFR | p50 5 ms, max 48 ms; ~7.6 KB for a 7-step reversal | 1 per RRN |
| 4.4 cards | no NFR | p50 5 ms, max 10 ms | 1 per RRN |
| 4.5 card | no NFR | p50 5 ms, max 14 ms | 1 per RRN |
| 4.6 ledger | no NFR | p50 6 ms, max 11 ms; ~9.5 KB per 200 journals | 1–10 per RRN |

Payload bounds: at most one step per code, so at most 11 steps; messages carry at most 12 fields; the ledger read is capped at 10 × 200 journals.

## 8. UI states

| Situation | What renders | Driving call |
| --- | --- | --- |
| `/transactions`, list loading | Header and tabs only | 4.1 |
| `/transactions`, no transaction of that status | "Chưa có giao dịch nào loại này", "Thực hiện một giao dịch ở Máy POS, hành trình của nó sẽ hiện ở đây.", link "Mở Máy POS" | 4.1 |
| `/transactions`, list failed | "Không tải được hành trình. Thử tải lại trang." | 4.1 |
| Journey loading | Nothing below the header | 4.2 |
| Journey failed (any error) | "Không tìm thấy giao dịch {rrn}." | 4.2 (JRN-G9) |
| Journey with 0 steps | Nothing below the header | 4.2 |
| Loaded | Playback starts on the last step; "Tự phát" replays from step 1 at 950 ms per step | 4.2 |
| Ledger unavailable or card not found | Money panel shows only the provider's debit/credit rows, no before/after balances | 4.4–4.6 |
| DECLINED | Money panel empty; note "Ngân hàng phát hành từ chối nên không có bút toán nào: tiền của khách không đổi." | 4.2 |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| JRN-G1 | ~~Journeys of pre-auth, completion, refund and balance inquiry show "declined before reaching the issuer" although they were sent and answered~~ | **Fixed** (#114): `advtxn` records `sent_at`, the network STAN, the processing code, the POS entry mode, the card token, the MTI and the CREATED→SENT→final history | GW | Done |
| JRN-G2 | The journey builder is purchase-only | `mtiRequest = "0200"` and a hard-coded `"0210"` in `message.go`, so a PREAUTH shows 0200/0210 instead of 0100/0110 and a COMPLETION instead of 0220/0230; POS_REQUEST's text says `POST /v1/purchases` (the path is `/v1/transactions/purchases`); `moneyRows` gives a REFUND a negative delta (`Delta: -txn.Amount`) | GW | Take the MTI pair and the money sign from `tran_type`; fix the path text |
| JRN-G3 | `Transaction` detail fields are never filled | `transactionDTO` types `approvedAmount` and `balance` as `*string` and never sets them; `originalRrn` isn't persisted (live COMPLETION `626807000294`: `originalRrn: null`); `traceId` is the RRN (`toTransactionDTO` ponytail comment) | GW | Persist approved amount, balance and original RRN in `tran_log`; add a `trace_id` column |
| JRN-G4 | `money[].balanceAfter` is always `null` | MCN-304-AC3 asks for issuer-reported balances (DE 54) or null; plan MCN-304 Ruling 7 keeps null, and the web reads the issuer ledger instead (MCN-307 Ruling 3), costing 3–12 extra requests per journey | GW + ISS | Keep the ruling, and add an `rrn` filter to `/v1/cards/{cardRef}/ledger` so the web needs one request |
| JRN-G5 | No live refresh for unfinished journeys | `useJourney` has no `refetchInterval`; the page uses no WS event | WEB (+ GW OVW-G3) | Poll every 2 s while `status` ∈ {SENT, TIMED_OUT, REVERSAL_PENDING}, or subscribe to `transaction.updated` once it exists |
| JRN-G6 | Query parameters aren't validated as the contract says | `?limit=500` returns 269 rows (max 200); `?cursor=zzz` → 500 `transactions-read-failed` (should be 400); `?status=BOGUS` → 200 empty; `/transactions/bad!!/journey` → 404 instead of 400 | GW | Validate `limit`, `status`, `rc`, `last4`, `cursor` and the RRN pattern in `parseTransactionFilter` / the handlers; 400 `validation-error` |
| JRN-G7 | The "Đã tự hủy" (auto-cancelled) tab opens any REVERSED transaction | Live: the newest REVERSED is `626807000345`, a MAC-failure reversal (RC 96, reason 06), which renders as outcome "cancelled" ("Giao dịch 10.000 ₫ đã được hủy"); POS cancellations (reason 17) land there too. `TransactionSummary` has no reversal reason to filter on | contracts + GW + WEB | Contract PR: add the reversal reason (DE 39 of the 0420) to `TransactionSummary` and a filter; the tab asks for reason 68 |
| JRN-G8 | "Mở trong phòng lab" opens the lab without the message | `IsoField.raw` / message bytes are never returned (messages are rebuilt, not captured); `StepDetail` links to `/lab/message` with no parameter | GW + WEB | Pack the rebuilt message and return it as `raw` on the MTI field, then pass it to the lab |
| JRN-G9 | Every journey error reads as "not found" | `JourneyView`: `if (isError) return <Notice>{t("notFound", { rrn })}</Notice>` for 404, 500 and 502 alike, after 3 retries | WEB | Distinguish 404 from other problems; don't retry a 404 |
| JRN-G10 | Problem responses aren't docs/04 §3 shaped | Bare slugs `unknown-transaction`, `transaction-read-failed`, `journey-read-failed`, `transactions-read-failed`, `invalid-request`; see OVW-G12 in [overview-page.md](overview-page.md) | GW | As OVW-G12 |
| JRN-G11 | An unknown `StepCode` isn't handled | `key()` in `components/journey/useJourneyCopy.ts` returns `step.{code}` for any code and calls `t()` without `t.has()`, so a code missing from `messages/*.json` shows the key path instead of the provider's `title`/`easyText` | WEB | Check the key with `t.has()` and fall back to the provider text |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version |
| 1.1 | 2026-09-25 | JRN-G1 fixed (#114) |
