# POS page: API contract

| | |
| --- | --- |
| Document | `docs/api/pos-page.md` |
| Version | 1.2 |
| Status | Approved for integration for purchases. Pre-authorization, completion, refund and balance inquiry are built but not proven against the real issuer (§9 POS-G6) |
| Date | 2026-09-25 |
| Screen | route `/pos`, container `web-next/src/app/(console)/pos/PosScreen.tsx` |
| Stories | MCN-305, MCN-604 (WEB); MCN-303, MCN-603 (GW); MCN-308 (ISS, card reads) |
| Provider(s) | gateway-go: `internal/api/purchases.go` → `internal/purchase`; `internal/api/advtxn.go` → `internal/advtxn`; errors in `internal/api/transaction_errors.go`. issuer-jpos Admin API: `adapter/http/CardAdminController.java` |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/pos-client.ts` and `src/shared/api/cards-client.ts` (`useCard`) |
| Design reference | canvas `project/POS.dc.html` (docs/01-prd.md §7.1); plan [`docs/plans/MCN-305-pos-canvas.md`](../plans/MCN-305-pos-canvas.md) |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25: GET calls only. POST examples are from the dev:mock handlers and are labelled |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and [`docs/04-api-contract.md`](../04-api-contract.md) §2–3, and are not repeated.

## 1. Purpose and scope

The POS page simulates a card terminal (TID `00000042`, "Cà phê Góc Phố"). The user picks one of six fixture cards, a transaction type and an entry mode, keys an amount and presses "Thanh toán". The page then shows the outcome, a plain explanation, the processing steps, and a link to the transaction's journey.

This contract covers the five transaction-creating calls and the card balance reads. Out of scope:
- `POST /v1/transactions/{rrn}/cancellations`: the gateway serves it, but the POS has no cancel action.
- PIN entry: the gateway never forwards a PIN block (risk R-12, plan Ruling R1).
- The journey itself: [journey-page.md](journey-page.md).

## 2. Page map

| UI region (label) | Data shown | Call(s) | Refresh |
| --- | --- | --- | --- |
| Title "Máy POS giả lập" + subtitle | static | – | – |
| "Loại giao dịch" selector: Mua hàng, Tiền ủy quyền trước, Hoàn tất, Hoàn tiền, Tra cứu số dư | Picks the call | §4.1–§4.5 | – |
| POS device: amount / "Số dư", terminal screen message | Keyed amount; after a balance inquiry, `balance.amount` | §4.5 | per payment |
| POS device: screen text ("Mời chạm, quẹt hoặc cắm thẻ", "Giao dịch thành công", …) | Outcome | §4.1–§4.5 | per payment |
| "Chọn thẻ để thanh toán": 6 card tiles | Tag, `•••• last4`, "Số dư {amount} ₫" | §4.6 | on mount; after each payment |
| "Cách đọc thẻ và kịch bản": entry mode, "Mã tra soát gốc", "Kịch bản nhanh" | Client state; the RRN is prefilled from an approved pre-auth | §4.2 response | – |
| "Kết quả" panel | Title, explanation, steps, Expert tech line, "Xem hành trình chi tiết" link | §4.1–§4.5, §4.6 | per payment |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `POST /v1/transactions/purchases` | gateway-go | Purchase (0200) | "Thanh toán", type Mua hàng | required; new UUID per press | none | Real |
| 4.2 | `POST /v1/transactions/pre-authorizations` | gateway-go | Pre-auth hold (0100) | "Thanh toán", type Tiền ủy quyền trước | required; new UUID per press | none | Real endpoint; issuer hold chain Planned MCN-601 (POS-G6) |
| 4.3 | `POST /v1/transactions/{rrn}/completions` | gateway-go | Completion advice (0220) | "Thanh toán", type Hoàn tất (disabled until an RRN is entered) | required; new UUID per press | none | Real endpoint; issuer chain Planned MCN-601 |
| 4.4 | `POST /v1/transactions/refunds` | gateway-go | Refund (0200, DE 3 `200000`) | "Thanh toán", type Hoàn tiền | required; new UUID per press | none | Real |
| 4.5 | `POST /v1/transactions/balance-inquiries` | gateway-go | Balance inquiry (0200, DE 3 `310000`) | "Thanh toán", type Tra cứu số dư | required; new UUID per press | none | Real endpoint; real issuer answers RC 30 (POS-G6) |
| 4.6 | `GET /v1/cards/{cardRef}` | issuer-jpos Admin API | Available balance per card tile and for the result text | on mount (6 calls); refetched after every payment | n/a | returns `ETag`, unused here | Real |
| – | `WS /v1/stream` | – | not consumed by this page | – | – | – | – |
| – | Terminal and card list | client-only | TID `00000042`, merchant name, 6 fixture cards (`DISPLAY_CARDS` in `components/pos/pos-model.ts`) | static | – | – | Client only (`GET /v1/terminals` is 404, POS-G11) |

## 4. Calls

### 4.1 POST /v1/transactions/purchases

- **Summary.** Builds, MACs and sends a 0200, waits up to 30 s for the 0210, and returns the transaction. A decline is not an HTTP error.
- **Request.**

| Header | Required | Value |
| --- | --- | --- |
| `Idempotency-Key` | yes | UUID. The client sends `crypto.randomUUID()` for each press of "Thanh toán" |
| `Content-Type` | yes | `application/json` |
| `traceparent` | no | forwarded by the BFF when present |

Body: `PurchaseRequest`.

| Field | Type | Required | Constraints (contract) | Notes |
| --- | --- | --- | --- | --- |
| `terminalId` | string | ✓ | exactly 8 chars | The page always sends `00000042`. Must be a terminal in the acquirer's `terminal` table |
| `cardToken` | string | ✓ | a token from `contracts/fixtures/cards.json` | `tok_normal`, `tok_low`, `tok_blocked`, `tok_expired`, `tok_limit`, `tok_second`. Never a PAN |
| `entryMode` | enum `EntryMode` | ✓ | `CHIP_PIN`, `CHIP_NO_PIN`, `MANUAL_PIN`, `MANUAL_NO_PIN` | The page sends only `CHIP_NO_PIN` ("Chạm chip", DE 22 `052`) and `MANUAL_NO_PIN` ("Nhập tay", `012`), Ruling R2 |
| `encryptedPinBlock` | string | – | `^[0-9A-F]{16}$` | Never sent by the page (R-12) |
| `emvData` | string | – | TLV hex | Never sent by the page |
| `amount.amount` | int64 | ✓ | ≥ 0, minor units | The keypad allows 1–10 digits; the page blocks 0 locally ("Số tiền chưa hợp lệ") |
| `amount.currency` | string | ✓ | `^[0-9]{3}$` | Always `"704"` |

- **Response.** `201`, schema `Transaction`, for every outcome (approved, declined, link-down, timed out).

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `status` | `TransactionStatus` | ✓ | Result icon, title, screen text | APPROVED → "Thanh toán thành công"; TIMED_OUT / REVERSAL_PENDING → "Giao dịch đang được tự động hủy"; REVERSED → "Giao dịch đã được tự động hủy" | same, plus tech line |
| `responseCode` | string | – (set when answered) | Decline title and "why" step | 51 "Tài khoản không đủ tiền", 54 "Thẻ đã hết hạn", 61 "Vượt hạn mức giao dịch", 62 "Thẻ đang bị khóa", 91 "Mất kết nối ngân hàng phát hành"; other → "Giao dịch bị từ chối" with "(mã {rc})" | `0200 STAN {stan} → 0210 · RC {rc}` |
| `authCode` | string | – | Expert tech line | not shown | `· field 38 = {authCode}` |
| `stan` | string | – | Expert tech line | not shown | `0200 STAN {stan}` |
| `rrn` | string | ✓ | "Xem hành trình chi tiết" → `/transactions/{rrn}` | link | link |
| `amount` | Money | ✓ | Result description | "Đã trừ {amount} ₫." | same |
| `maskedPan` | string | ✓ | Step "Máy POS gửi yêu cầu" (last four) | "Đọc chip thẻ •••• {last4}, không cần PIN." | same |
| `type` | enum | ✓ | Title/description variant and MTI pair | | `0200 → 0210` |
| `terminalId`, `merchantName`, `createdAt`, `businessDate`, `traceId` | | | not shown | | |

The balance in "Số dư còn {balance} ₫" comes from §4.6, not from this response.

- **Provider rules.**
  1. The Idempotency-Key must be a UUID (else 400 `insufficient-idempotency-key`). It is reserved atomically before anything is sent. A replay of the same body within 24 h returns the stored `Transaction` unchanged, without sending anything. The same key with a different body ⇒ 422 `idempotency-key-mismatch`. The same key while the first request is still in flight ⇒ 409 `conflict`. A key whose request failed before anything was sent is released and may be retried.
  2. `cardToken` resolves to a fixture card and `terminalId` to its merchant (DE 42) before anything is sent. Unknown terminal ⇒ 422 `unknown-terminal`; unknown token ⇒ 422 `unknown-card-token`. The body is validated first: `amount.amount` > 0, `amount.currency` `^[0-9]{3}$`, `entryMode` in the enum, `terminalId` 8 characters, `cardToken` present ⇒ else 400 `validation-error` with `errors[]`.
  3. Link not signed on, or no STAN available ⇒ nothing is sent; `status: DECLINED`, `responseCode: "91"`, STAN `000000` in the RRN (MCN-303-AC4). The row is still written to `tran_log`.
  4. The 0200 carries DE 2, 3 (`000000`), 4, 7, 11, 12, 13 (terminal local time, Asia/Ho_Chi_Minh), 14, 15, 22, 32 (`970499`), 37, 41, 42, 49 and a DE 64 MAC under the ZAK.
  5. `tran_log` moves CREATED → SENT → outcome, and each transition is written to `tran_state_history`.
  6. A 0210 with RC `00` ⇒ APPROVED; any other RC ⇒ DECLINED. A 0210 whose MAC fails (against the current ZAK and, for 5 minutes after a rotation, the retired one) is returned as DECLINED RC `96`, and a 0420 with reason `06` is queued in SAF.
  7. No 0210 within 30 s, or any other send failure after the write (broken connection, cancelled request) ⇒ the row goes TIMED_OUT, a 0420 with reason `68` is queued in SAF, and the response carries the row's real status, `REVERSAL_PENDING`. The 0200 is never resent (root CLAUDE.md §6.4). A link that drops before the write is DECLINED RC `91`.
  8. `maskedPan` is first 6 + last 4. `businessDate` is the UTC date of the request.
  9. One `transaction.created` WS event is broadcast with the returned transaction.
- **Errors.** Problem `type` values are bare slugs today (OVW-G12 in [overview-page.md](overview-page.md)).

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `insufficient-idempotency-key` | Header missing or not a UUID (never, from this page) | "Không gửi được giao dịch" + "Máy chủ trả lỗi: {detail}" |
| 400 | `validation-error` | Body is not valid JSON or breaks the schema; `errors[]` names each field | same |
| 409 | `conflict` | The same Idempotency-Key is still in flight | same |
| 422 | `unknown-terminal` | `terminalId` not in the acquirer's terminals | same |
| 422 | `unknown-card-token` | `cardToken` is not a simulator card | same |
| 422 | `idempotency-key-mismatch` | The Idempotency-Key was used for a different body | same |
| 500 | `purchase-failed` | Any other error: a database error | same |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | BFF can't reach the gateway | same |

- **Example.** dev:mock (`web-next/src/mocks/scenario-handlers.ts`, scenario "Mua hàng bình thường"):

```http
POST /api/v1/transactions/purchases
Idempotency-Key: <uuid>
Content-Type: application/json

{ "terminalId": "00000042", "cardToken": "tok_normal", "entryMode": "CHIP_NO_PIN",
  "amount": { "amount": 250000, "currency": "704" } }
```

```json
{
  "rrn": "626807000124", "stan": "000124", "type": "PURCHASE",
  "status": "APPROVED", "responseCode": "00", "authCode": "A00124",
  "amount": { "amount": 250000, "currency": "704" }, "balance": null,
  "maskedPan": "970436******4417", "terminalId": "00000042",
  "merchantName": "Cà phê Góc Phố", "createdAt": "2026-09-25T09:40:12.511Z"
}
```

  The real gateway returns the same shape plus `businessDate`, `responseLabel`, `latencyMs` and `traceId` (the request span's W3C trace id, also stored in `tran_log.trace_id`), without `balance`, and omits `responseCode`/`authCode` when they are empty. For comparison, the stored result of a real approved purchase, read back with `GET /v1/transactions/626807000352`: `status APPROVED`, `responseCode "00"`, `authCode "L7AFDG"`, `latencyMs 8059`.
- **Notes.** Mutations don't retry (TanStack default). The pay button is locked while any mutation is pending. The BFF sets no timeout; the gateway's HTTP write timeout is 35 s, above the 30 s ISO timeout. Rate limit: not enforced.

### 4.2 POST /v1/transactions/pre-authorizations

- **Summary.** Sends a 0100 with DE 25 `06` to place a hold.
- **Request.** Same headers and body as §4.1 (`PurchaseRequest`).
- **Response.** `201`, `Transaction`. Fields as §4.1, plus:

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `rrn` (when APPROVED) | string | ✓ | Prefills "Mã tra soát gốc" for a later completion | "Đã tạm giữ {amount} ₫ trên thẻ. Dùng mã tra soát {rrn} để hoàn tất." | same |
| `approvedAmount` | Money | – | not shown | | |

  Title on approval: "Đã giữ tiền thành công". Tech line MTI pair: `0100 → 0110`.
- **Provider rules.**
  1. Idempotency as §4.1 rule 1, route `pre-authorizations`, but a STAN and RRN are allocated before the replay check.
  2. The 0100 carries DE 2, 3 (`000000`), 4, 7, 11, 14, 22, 25 (`06`), 32, 37, 41, 42, 49 and a DE 64 MAC.
  3. RC `00` or `10` ⇒ APPROVED; RC `10` also sets `approvedAmount` from DE 4 (partial approval). Any other RC ⇒ DECLINED.
  4. As a purchase (§4.1 rules 5–7): a link that is down records DECLINED RC `91`; any send failure after the write queues a reason-`68` 0420 and returns `REVERSAL_PENDING`; a bad incoming MAC (checked against the 0110) is DECLINED RC `96` with a reason-`06` 0420. A refund does the same. A **completion** is an advice and is never reversed (docs/03 §7.4): on any send failure after the write, or a 0230 that fails its MAC, the row goes `TIMED_OUT`, the response says `TIMED_OUT`, and the 0220 itself is stored in SAF and repeated as 0221 (same STAN and DE 7, a fresh DE 64 MAC, no DE 2) until a 0230 RC `00`, which moves the row to APPROVED. A **balance inquiry** holds no money, so a timeout leaves it `TIMED_OUT` and a bad MAC leaves it DECLINED RC `96`, with nothing queued.
  5. `sent_at`, the network STAN, the processing code, the POS entry mode, the card token, the MTI and `tran_state_history` are recorded, so the journey and a 0420 are built from real data. DE 90 of the 0420 names the original MTI.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `insufficient-idempotency-key` | Header missing or not a UUID | "Không gửi được giao dịch" + detail |
| 400 | `validation-error` | Body is not valid JSON or breaks the schema; `errors[]` names each field | same |
| 422 | `unknown-terminal` | Unknown `terminalId` | same |
| 500 | `pre-authorization-failed` | A failed reversal enqueue, a database error | same |
| 502 | `…/upstream-unavailable` (BFF) | Gateway unreachable | same |

- **Example.** dev:mock: request as §4.1 with `"amount": { "amount": 500000, "currency": "704" }`; response `{"rrn":"626807000125","stan":"000125","type":"PREAUTH","status":"APPROVED","responseCode":"00","authCode":"A00125","amount":{"amount":500000,"currency":"704"},"balance":null,"maskedPan":"970436******4417","terminalId":"00000042","merchantName":"Cà phê Góc Phố","createdAt":"…"}`. Real stored result for comparison (GET): `626807000293`, PREAUTH, DECLINED, RC `62`, card ••••3310 (blocked).
- **Notes.** As §4.1.

### 4.3 POST /v1/transactions/{rrn}/completions

- **Summary.** Sends a 0220 completing the pre-authorization `{rrn}`, at the pre-auth's terminal and card.
- **Request.**

| Parameter | In | Type | Required | Constraints |
| --- | --- | --- | --- | --- |
| `rrn` | path | string | ✓ | `^[0-9A-Z]{12}$`; the page takes it from "Mã tra soát gốc" (prefilled after an approved pre-auth, or typed) |

  Headers as §4.1. Body:

| Field | Type | Required | Constraints | Notes |
| --- | --- | --- | --- | --- |
| `amount.amount` | int64 | ✓ | ≥ 0 | The keyed amount |
| `amount.currency` | string | ✓ | `^[0-9]{3}$` | `"704"` |

- **Response.** `201`, `Transaction`. As §4.1, plus `originalRrn` (the pre-auth's RRN), shown in the step "Máy POS gửi yêu cầu": "Dùng mã tra soát {rrn} của lần giữ tiền." The completion gets its own new `rrn`. Title on approval: "Đã hoàn tất giao dịch"; MTI pair `0220 → 0230`.
- **Provider rules.**
  1. `{rrn}` must exist in `tran_log` (else 404 `not-found`) and be a PREAUTH in APPROVED (else 409 `conflict`). The gateway doesn't yet check that the amount is within the hold, or that the pre-auth wasn't already completed. The original RRN is part of the idempotency hash.
  2. The 0220 carries DE 3, 4, 7, 11, 32, 37 (= the pre-auth's RRN), 49 and a DE 64 MAC; no DE 2 or DE 42.
  3. RC mapping, timeout, MAC and history behaviour as §4.2 rules 3–5.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `insufficient-idempotency-key` / `validation-error` | as §4.2, and a malformed `{rrn}` | "Không gửi được giao dịch" + detail |
| 404 | `not-found` | `{rrn}` not in `tran_log` | same. dev:mock differs: it returns 201 DECLINED RC `25` (a WEB change, see §9 POS-G13) |
| 409 | `conflict` | `{rrn}` is not an APPROVED PREAUTH | same |
| 422 | `unknown-terminal` | The pre-auth's terminal no longer resolves | same |
| 500 | `completion-failed` | A failed reversal enqueue, a database error | same |

- **Example.** dev:mock: `POST /api/v1/transactions/626807000125/completions` with `{"amount":{"amount":500000,"currency":"704"}}` → `{"rrn":"626807000126","type":"COMPLETION","status":"APPROVED","responseCode":"00","originalRrn":"626807000125",…}`. Real stored result for comparison (GET): `626807000294`, COMPLETION, DECLINED, `responseCode: null`, `originalRrn: null` (POS-G6, JRN-G3).

### 4.4 POST /v1/transactions/refunds

- **Summary.** Sends a 0200 with DE 3 `200000` crediting the card.
- **Request.** As §4.1 (`PurchaseRequest`).
- **Response.** `201`, `Transaction`, as §4.1. Title on approval "Hoàn tiền thành công", description "Đã hoàn {amount} ₫ vào tài khoản của khách."; MTI pair `0200 → 0210`.
- **Provider rules.** As §4.2 rules 1 and 3–5 (route `refunds`). The 0200 carries DE 2, 3 (`200000`), 4, 7, 11, 14, 22, 32, 37, 41, 42, 49 and DE 64.
- **Errors.** As §4.2, with 500 type `refund-failed`.
- **Example.** dev:mock: request as §4.1 → `{"type":"REFUND","status":"APPROVED","responseCode":"00",…}`; the mock adds the amount to the card's in-memory balance.

### 4.5 POST /v1/transactions/balance-inquiries

- **Summary.** Sends a 0200 with DE 3 `310000` and no amount; returns the available balance from DE 54.
- **Request.** Headers as §4.1. Body `CardPresentData`: `terminalId`, `cardToken`, `entryMode` (as §4.1); no `amount`. The keypad is disabled for this type.
- **Response.** `201`, `Transaction`, plus:

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `balance` | Money | – (set when DE 54 parses) | POS device "Số dư" value; description | "Số dư khả dụng là {balance} ₫." ("—" on the device when absent) | same |

- **Provider rules.**
  1. As §4.2 rules 1 and 3–5 (route `balance-inquiries`).
  2. `balance` is parsed from DE 54 as currency (3) + D/C (1) + amount (12 digits); any other length leaves it `null`.
  3. The response `amount` is `{ "amount": 0, "currency": "704" }`: the card's currency from `contracts/fixtures/cards.json`, also stored in `tran_log.currency`. `balance` is stored in `tran_log.balance_amount`/`balance_currency` and returned by the GET detail.
- **Errors.** As §4.2, with 500 type `balance-inquiry-failed`.
- **Example.** dev:mock: `{"type":"BALANCE","status":"APPROVED","responseCode":"00","amount":{"amount":0,"currency":"704"},"balance":{"amount":5000000,"currency":"704"},…}`. Real stored result (GET `/v1/transactions/626807000291`): `"status":"DECLINED","responseCode":"30","amount":{"amount":0,"currency":"   "}`.

### 4.6 GET /v1/cards/{cardRef}

- **Summary.** The issuer's card detail. The POS reads only `availableBalance`.
- **Request.** Path `cardRef` (`^crd_[A-Za-z0-9]{10,32}$`), from `DISPLAY_CARDS`: `crd_normal0001`, `crd_lowbal0002`, `crd_blockd0003`, `crd_expird0004`, `crd_limit00005`, `crd_second0006`.
- **Response.** `200`, `CardDetail`, with an `ETag` header (used by the Cards screen for limit updates, not here).

| Field | Type | Req. | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `availableBalance.amount` | int64 | ✓ | Card tile "Số dư {amount} ₫"; result description "Số dư còn {balance} ₫" / "Số dư {balance} ₫ thấp hơn…" | vi-VN grouping | same |
| everything else | | | not shown on this page | | |

- **Provider rules.** `availableBalance` is the issuer account's available balance after every posted journal. `holds` is always `[]` today (no issuer pre-auth chain, POS-G6), and on the local stack `availableBalance` equals `ledgerBalance` for every card.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | `https://mcn.local/problems/not-found` | Unknown `cardRef` | Tile shows no balance line; the result omits the balance |
| 502 | `…/upstream-unavailable` (BFF) | Issuer Admin API unreachable | same |

- **Example.** Real (local stack, through the BFF):

```json
{
  "cardRef": "crd_normal0001", "maskedPan": "970436******4417", "holderName": "Nguyen Minh Anh",
  "status": "ACTIVE", "expiry": "11/28",
  "ledgerBalance": { "amount": 2982000, "currency": "704" },
  "availableBalance": { "amount": 2982000, "currency": "704" },
  "holds": [],
  "limits": { "dailyAmount": { "amount": 8000000, "currency": "704" }, "perTransactionAmount": { "amount": 9500000, "currency": "704" }, "dailyCount": null },
  "usedToday": { "amount": 2018000, "currency": "704" }
}
```

- **Notes.** Client: `useCard(cardRef)`, query key `["cards", cardRef]`, shared with the Cards screen. After every payment the page invalidates `["cards"]`, so all six tiles refetch. TanStack defaults: 3 retries, refetch on window focus.

## 5. Real-time events

None. The page shows the synchronous POST result. A reversal that completes later (REVERSAL_PENDING → REVERSED) isn't reflected on this page; the journey page shows it on reload.

## 6. Security and compliance

- The page never holds a PAN: cards are addressed by `cardToken` (requests) and `cardRef` (reads), and displayed as `•••• last4`. `DISPLAY_CARDS` carries token, reference and last four only; the fixture `pan` is never embedded in web source.
- Responses carry `maskedPan` (first 6 + last 4) at most. The gateway masks with `obs.MaskPAN` before the value reaches `tran_log` or a response.
- No PIN, PIN block or track data is sent or returned. `encryptedPinBlock` is never sent (R-12, Ruling R1); MCN-305-AC4 returns when R-12 closes.
- Every POST carries an `Idempotency-Key` (a UUID). The gateway reserves it atomically and compares the stored request hash, so a reused key with a different body is 422 `idempotency-key-mismatch` and two concurrent requests with one key send once.
- Money: integer minor units, currency `"704"`.
- Audit: the gateway records every transaction in `tran_log` and its transitions in `tran_state_history`. No card-admin writes happen on this page.
- 500 responses carry the raw Go error in `detail`, which the page shows verbatim in "Máy chủ trả lỗi: {detail}". No card data has been seen in those strings, but nothing prevents it (OVW-G12).

## 7. Non-functional requirements

| Call | Budget | Observed on the local stack (2026-09-25) | Load from this page |
| --- | --- | --- | --- |
| 4.1 purchase | NFR-01: end-to-end p99 < 300 ms at 200 TPS; hard ceiling 30 s (ISO timeout) | Overview `p99LatencyMs` 25 ms, `p50LatencyMs` 3 ms (SENT → outcome, today); stored `latencyMs` typically 20–36 ms | 1 per press |
| 4.2–4.5 | same ceiling | not measured (POST not sent) | 1 per press |
| 4.6 card | no NFR | p50 5 ms, max 14 ms (30 samples via the BFF) | 6 on mount, 6 after each payment |

Payload bounds: requests < 300 bytes; `Transaction` ≈ 450 bytes; `CardDetail` ≈ 500 bytes. No pagination.

## 8. UI states

| Situation | What renders | Driving call |
| --- | --- | --- |
| Initial | Result panel "Thực hiện giao dịch đầu tiên" + hint; device screen "Mời chạm, quẹt hoặc cắm thẻ" | – |
| Amount 0 | "Số tiền chưa hợp lệ", screen "Nhập số tiền"; nothing is sent | client check |
| In flight | Spinner, "Đang gửi tới ngân hàng phát hành…"; Expert: "{request} đã gửi · MUX chờ {response} (timeout 30 s)"; pay button locked | §4.1–§4.5 |
| Approved / declined / reversal | Result per §4.1 table; steps; "Xem hành trình chi tiết" | §4.1–§4.5 |
| HTTP error | "Không gửi được giao dịch", "Máy chủ trả lỗi: {detail}", screen "Giao dịch bị từ chối" | any problem response |
| Card balance unavailable | Tile without its balance line; result text without the balance | §4.6 |
| Completion without RRN | Pay button disabled | client check |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| POS-G1 | ~~Unknown `cardToken` is a 500, not a 4xx~~ | **Fixed** (#PR2): `transactionProblem` maps `purchase.ErrUnknownCardToken` to 422 `unknown-card-token` | GW | Done |
| POS-G2 | ~~No request validation~~ | **Fixed** (#PR2): `internal/api/validation.go` checks every POST body and path RRN; 400 `validation-error` with `errors[]` | GW | Done |
| POS-G3 | ~~Idempotency is weaker than docs/04 §2~~ | **Fixed** (#PR2): `IdempotencyRepository.Reserve` (INSERT … ON CONFLICT, 24 h TTL, request-hash compare, 409 while in flight); UUID keys; released only when nothing was sent; `advtxn` checks the replay before allocating a STAN | GW | Done |
| POS-G4 | ~~Advanced transactions break "unknown outcome ⇒ reversal" (root CLAUDE.md §6.4)~~ | **Fixed** (#114): every send failure after the write marks the row TIMED_OUT and queues a 0420 (DE 39 68) for purchases, pre-auths and refunds; a completion's 0220 is repeated as 0221 until its 0230 instead (docs/03 §7.4); a balance inquiry owes nothing. Link down, or a link that drops before the write, records DECLINED RC 91 (`purchase.SendFailureStatus`). Everything after the send runs on a detached context with a 10 s bound (`purchase.Detach`), and the follow-up is queued from the row's real state even when the status update fails | GW | Done |
| POS-G5 | ~~Incoming MAC not verified for advanced transactions~~ | **Fixed** (#114): `advtxn` verifies the incoming MAC through the shared `purchase.MACVerifier`; a mismatch is DECLINED RC 96 with a reason-06 reversal | GW | Done |
| POS-G6 | Pre-auth, completion and balance inquiry aren't proven against the real issuer | Package doc of `internal/advtxn/service.go` (Ruling 1: proven against `internal/chaos/fakeissuer`); live rows: BALANCE `626807000291` DECLINED RC 30, COMPLETION `626807000294` DECLINED with `responseCode: null`; `CardAdminController.cardDetail` always returns `holds: []` | ISS + GW | MCN-601 (issuer hold and completion chain); a DECLINED row always carries an RC. Issuer side proven in #119: a balance inquiry without DE 4 is approved with DE 54 (it used to get RC 30), and a refund credits the account |
| POS-G7 | ~~Balance inquiry breaks the `Money` pattern~~ | **Fixed** (#PR2): a balance inquiry's amount is `{0, "704"}` from the card fixture's currency | GW | Done |
| POS-G8 | ~~POST responses miss fields the schema defines~~ | **Fixed** (#PR2): POST responses carry `responseLabel`, `latencyMs` and the request span's `traceId`, stored in `tran_log.trace_id` (migration 00007) and returned by the GET detail | GW | Done |
| POS-G9 | ~~Timeout response says TIMED_OUT while the row is REVERSAL_PENDING~~ | **Fixed** (#114): the timeout response reports REVERSAL_PENDING | GW + docs | Done |
| POS-G10 | docs/04 §3 lists `link-down` 503; the gateway returns 201 DECLINED RC 91 | `CreatePurchase` link-down branch; MCN-303-AC4 | docs | Doc-fix PR: drop `link-down` 503 from docs/04 §3 (the story and the code agree) |
| POS-G11 | `GET /v1/terminals` is in the contract but not served | `curl :8080/v1/terminals` → 404; the page hard-codes TID `00000042` and "Cà phê Góc Phố" (`PosScreen.tsx` `TERMINAL`) | GW | Implement the route from the `terminal`/`merchant` tables; let the POS pick a terminal |
| POS-G12 | No PIN entry (MCN-305-AC4 deferred) | Plan Ruling R1; the gateway never forwards DE 52 (R-12) | GW + WEB | Close R-12, then restore the PIN pad and `encryptedPinBlock` |
| POS-G13 | ~~Completion accepts any known RRN; the mock and the gateway disagree on an unknown one~~ | **Fixed** (#PR2): the gateway answers 409 `conflict` unless the original is an APPROVED PREAUTH, 404 `not-found` when it is unknown. The dev:mock still answers 201 DECLINED RC 25 (WEB lane, not changed here) | GW + WEB | Done (dev:mock parity in #125; the mock answers `unknown-transaction`, the gateway now `not-found`) |
| POS-G14 | A retry after an HTTP failure creates a new Idempotency-Key | `pos-client.ts` calls `crypto.randomUUID()` inside each `mutationFn`; after a 500/502 whose outcome is unknown, pressing "Thanh toán" again can charge twice | WEB | **Fixed** in #125: the key belongs to the draft (`useDraftMutation`); the same draft retried after an unknown outcome resends it, a changed draft or a success mints a new one |
| POS-G15 | Link-down declines share one RRN within an hour | A decline that was never sent uses STAN `000000`, so every link-down decline in the same hour gets RRN `YDDDHH000000` (`purchase.declinedTransaction`, `advtxn.noSTAN`); `GET /v1/transactions/{rrn}` then returns whichever row it finds first | GW | Reserve a real STAN (or a separate local sequence) for link-down declines |
| POS-G16 | No sweeper for orphan SENT / TIMED_OUT rows | If the gateway dies between the send and recording the outcome, the row stays SENT (or TIMED_OUT with nothing queued) forever; nothing reverses it on restart. The same short windows exist after commit: between the send and the status update (#114 review N1), and between the status update and the SAF insert (N2); a crash in either leaves the row unreversed | GW | A startup/periodic sweeper that queues a 0420 (or a 0221 repeat for a completion) for rows older than the send timeout |
| POS-G17 | The issuer debited the account for every processing code, so a refund took money out of the customer's account | #114 review; `Authorize` always ran the purchase debit | ISS | **Fixed** in #119: a refund credits the customer (REFUND journal D `SETTLEMENT_SUSPENSE` / C customer, no funds or velocity check), and its reversal debits them back, even past the overdraft floor (Ruling R-1: reversals always post; a `NEGATIVE_BALANCE_AFTER_REVERSAL` audit row flags it). A duplicate balance inquiry replays its stored DE 54 and DE 38 (`tran_log.balance`, R-2) |
| POS-G18 | The issuer can't process a completion advice (0220/0221) | The issuer's class-02 participant chain throws in CheckCard when there is no DE 2 or PAN_HASH, and Respond answers with DE 39 null. So a completion's direct reply is DECLINED with no RC, and a repeated 0221 in SAF is never acknowledged and goes DEAD. Also non-standard: DE 37 of the 0220 is the pre-auth's RRN, not the completion's own | ISS | MCN-601: issuer completion handling locates the pre-auth, posts, never declines, and dedupes x21 against x20 |
| POS-G19 | Reversals never restored account balances: `LocateAndReverse` posted the reversing journal but never updated `account.ledger_balance`/`available_balance`, so every reversed purchase left the customer debited while the ledger said the money came back. Live stack: `crd_second0006` read 169 446 500 against 170 253 500 from its postings, a gap of 807 000 = its three reversals (235 500 + 321 500 + 250 000); accounts without reversals matched. Everything that reads these columns was wrong after a reversal: the Card Admin balances (cards-page.md §4.2), the web Journey money panel, and the chaos ledger check (CHA-G1, fixed in the gateway by #118; it reads these balances, so it only agrees with the ledger once #119 lands) | `LocateAndReverse` / `LedgerRepository.postReversal` had no `UPDATE account` | ISS | **Fixed** in #119: the reversal mirrors the original journal (directions swapped) and moves the balance through the same `AccountLockRepository.adjust` Authorize uses, in the transaction that marks the row `REVERSED`. The reversal of a debit also takes it off the day's `velocity_counter` (count and amount, R-3), so a reversed purchase frees its limit. An approval commits its `APPROVED` row with the money (S1), and a reversal abandons an original stuck `RECEIVED` past the timeout with no money moved as `REVERSED` (0430 `00`), so a crashed issuer no longer leaves the SAF repeating 96 until the advice goes DEAD; a late approval for that row rolls back with RC 94 (docs/05 §3 `tran_log` RECEIVED → outcome). Tests: every account ends at opening + Σ(credits − debits); a 200-ordering property test for purchases and refunds. Running stacks need a reseed, since no data fix is shipped |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version |
| 1.1 | 2026-09-25 | POS-G4, POS-G5 and POS-G9 fixed (#114); POS-G15 to POS-G18 recorded from the #114 security review |
| 1.2 | 2026-09-25 | POS-G17 (refunds credit) marked Fixed and POS-G19 (reversals never restored balances) added as Fixed, both in #119; balance inquiry approved with DE 54 (POS-G6 note); rulings R-1 (reversals always post, may overdraw), R-2 (duplicate inquiry replays DE 54), R-3 (reversal frees velocity). |
| 1.3 | 2026-09-26 | POS-G14 fixed, POS-G13 mock parity (#125) |
| 1.4 | 2026-09-25 | POS-G1, POS-G2, POS-G3, POS-G7, POS-G8 and POS-G13 fixed (#PR2) |
