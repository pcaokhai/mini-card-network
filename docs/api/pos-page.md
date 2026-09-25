# POS page: API contract

| | |
| --- | --- |
| Document | `docs/api/pos-page.md` |
| Version | 1.1 |
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
  1. An Idempotency-Key already stored for the route `purchases` returns the stored `Transaction` unchanged, without sending anything. The stored response has no expiry.
  2. `cardToken` resolves to a fixture card and `terminalId` to its merchant (DE 42) before anything is sent. Unknown terminal ⇒ 422; unknown token ⇒ 500 (POS-G1).
  3. Link not signed on, or no STAN available ⇒ nothing is sent; `status: DECLINED`, `responseCode: "91"`, STAN `000000` in the RRN (MCN-303-AC4). The row is still written to `tran_log`.
  4. The 0200 carries DE 2, 3 (`000000`), 4, 7, 11, 12, 13 (terminal local time, Asia/Ho_Chi_Minh), 14, 15, 22, 32 (`970499`), 37, 41, 42, 49 and a DE 64 MAC under the ZAK.
  5. `tran_log` moves CREATED → SENT → outcome, and each transition is written to `tran_state_history`.
  6. A 0210 with RC `00` ⇒ APPROVED; any other RC ⇒ DECLINED. A 0210 whose MAC fails (against the current ZAK and, for 5 minutes after a rotation, the retired one) is returned as DECLINED RC `96`, and a 0420 with reason `06` is queued in SAF.
  7. No 0210 within 30 s ⇒ the response carries `status: TIMED_OUT` and a 0420 with reason `68` is queued in SAF. The 0200 is never resent (root CLAUDE.md §6.4). The `tran_log` row is REVERSAL_PENDING by the time the response is written (POS-G9).
  8. `maskedPan` is first 6 + last 4. `businessDate` is the UTC date of the request.
  9. One `transaction.created` WS event is broadcast with the returned transaction.
- **Errors.** Problem `type` values are bare slugs today (OVW-G12 in [overview-page.md](overview-page.md)).

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `idempotency-key-required` | Header missing (never, from this page) | "Không gửi được giao dịch" + "Máy chủ trả lỗi: {detail}" |
| 400 | `invalid-request` | Body is not valid JSON | same |
| 422 | `unknown-terminal` | `terminalId` not in the acquirer's terminals | same |
| 500 | `purchase-failed` | Any other error: unknown `cardToken`, a send error other than the timeout, a database error | same. **The outcome may be unknown**, see POS-G4 |
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

  The real gateway returns the same shape plus `businessDate` and `traceId` (empty string today, POS-G8), without `balance`, and omits `responseCode`/`authCode` when they are empty. For comparison, the stored result of a real approved purchase, read back with `GET /v1/transactions/626807000352`: `status APPROVED`, `responseCode "00"`, `authCode "L7AFDG"`, `latencyMs 8059`.
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
  4. There is no link-down path and no timeout path: a send error, including the 30 s timeout, is a 500, the `tran_log` row stays SENT, and no reversal is queued (POS-G4). The incoming MAC isn't verified (POS-G5).
  5. `tran_state_history` isn't written and `sent_at` isn't set, so the journey of this transaction is wrong ([journey-page.md](journey-page.md) JRN-G1).
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `idempotency-key-required` | Header missing | "Không gửi được giao dịch" + detail |
| 400 | `invalid-request` | Body is not valid JSON | same |
| 422 | `unknown-terminal` | Unknown `terminalId` | same |
| 500 | `pre-authorization-failed` | Unknown `cardToken`, no STAN, send error or timeout, database error | same |
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
  1. `{rrn}` must exist in `tran_log`; otherwise 404. The gateway doesn't check that it is an approved PREAUTH, or that the amount is within the hold (POS-G13).
  2. The 0220 carries DE 3, 4, 7, 11, 32, 37 (= the pre-auth's RRN), 49 and a DE 64 MAC; no DE 2 or DE 42.
  3. RC mapping, timeout, MAC and history behaviour as §4.2 rules 3–5.
- **Errors.**

| HTTP status | Problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `idempotency-key-required` / `invalid-request` | as §4.2 | "Không gửi được giao dịch" + detail |
| 404 | `unknown-transaction` | `{rrn}` not in `tran_log` | same. dev:mock differs: it returns 201 DECLINED RC `25` (POS-G13) |
| 422 | `unknown-terminal` | The pre-auth's terminal no longer resolves | same |
| 500 | `completion-failed` | No STAN, send error or timeout, database error | same |

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
  3. The response `amount` is `{ "amount": 0, "currency": "" }` and the stored row's currency is blank, which breaks the `Money` pattern (POS-G7).
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
- Every POST carries an `Idempotency-Key`. The gateway doesn't compare the stored request hash, so a reused key with a different body silently returns the first result (POS-G3).
- Money: integer minor units, currency `"704"`.
- Audit: the gateway records every transaction in `tran_log` and its transitions in `tran_state_history` (purchases only, JRN-G1). No card-admin writes happen on this page.
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
| POS-G1 | Unknown `cardToken` is a 500, not a 4xx | `transactionProblem` in `internal/api/transaction_errors.go` has no case for `purchase.ErrUnknownCardToken`; falls to `purchase-failed` / `<type>-failed` 500 | GW | Map it to 422 (or 400 `validation-error` with `errors[]`) |
| POS-G2 | No request validation | Handlers only JSON-decode; `amount ≤ 0`, a negative amount, a non-numeric currency, an `entryMode` outside the enum (DE 22 left empty) or a `terminalId` of the wrong length reach the service. docs/04 §3 requires 400 `validation-error` with `errors[]` | GW | Validate `PurchaseRequest`/`CardPresentData` at the handler against the schema |
| POS-G3 | Idempotency is weaker than docs/04 §2 | `idempotency.Find` returns the stored body without comparing `request_hash` (no 422 `idempotency-key-mismatch`); the key isn't checked to be a UUID; records never expire (docs/04: 24 h); Find-then-Store isn't atomic, so two concurrent requests with one key both send; `advtxn` allocates a STAN before the replay check | GW | Compare hashes, `INSERT … ON CONFLICT` reservation before sending, TTL. The issuer's `CardAdminController.replayIfPresent` already compares hashes |
| POS-G4 | Advanced transactions break "unknown outcome ⇒ reversal" (root CLAUDE.md §6.4) | `advtxn.Service.send`: any `mux.Send` error, including the 30 s timeout, returns `fmt.Errorf("send %s: %w")` → 500; the `tran_log` row stays SENT and no 0420 is queued. Purchases also return 500 on a non-timeout send error (`mapSendOutcome`) with no reversal. No `IsSignedOn` check in `advtxn`, so link-down is a 500 instead of DECLINED RC 91 | GW | Reuse the purchase timeout branch (`finalizeSendResult` + `ReversalQueuer.Queue`) for every flow and every ambiguous send error; add the link-down path |
| POS-G5 | Incoming MAC not verified for advanced transactions | `purchase.Service.verifyIncomingMAC` has no counterpart in `internal/advtxn` | GW | Share the verification (and the RC 96 + reason 06 reversal) with `advtxn` |
| POS-G6 | Pre-auth, completion and balance inquiry aren't proven against the real issuer | Package doc of `internal/advtxn/service.go` (Ruling 1: proven against `internal/chaos/fakeissuer`); live rows: BALANCE `626807000291` DECLINED RC 30, COMPLETION `626807000294` DECLINED with `responseCode: null`; `CardAdminController.cardDetail` always returns `holds: []` | ISS + GW | MCN-601 (issuer hold and completion chain); a DECLINED row always carries an RC. Issuer side proven in #119: a balance inquiry without DE 4 is approved with DE 54 (it used to get RC 30), and a refund credits the account |
| POS-G7 | Balance inquiry breaks the `Money` pattern | `CreateBalanceInquiry` passes no `requestedAmt`, so the response has `currency: ""` and `tran_log.currency` is blank: live `"amount":{"amount":0,"currency":"   "}` | GW | Use the card's currency (`"704"`) for the zero amount |
| POS-G8 | POST responses miss fields the schema defines | `purchase.Transaction` has no `responseLabel`/`latencyMs` and never sets `TraceID` (always `""`); `advtxn.Transaction` has no `traceId`, `responseLabel` or `latencyMs`. The GET detail fills `responseLabel`/`latencyMs` and uses the RRN as `traceId` | GW | One DTO for POST and GET; persist a real trace id (NFR-08) |
| POS-G9 | Timeout response says TIMED_OUT while the row is REVERSAL_PENDING | `finalizeSendResult` returns the queue-time status; docs/04 §4 says "returns final or REVERSAL_PENDING state". No UI impact: `outcomeOf` treats both as `reversalPending` | GW + docs | Return REVERSAL_PENDING, or amend docs/04 |
| POS-G10 | docs/04 §3 lists `link-down` 503; the gateway returns 201 DECLINED RC 91 | `CreatePurchase` link-down branch; MCN-303-AC4 | docs | Doc-fix PR: drop `link-down` 503 from docs/04 §3 (the story and the code agree) |
| POS-G11 | `GET /v1/terminals` is in the contract but not served | `curl :8080/v1/terminals` → 404; the page hard-codes TID `00000042` and "Cà phê Góc Phố" (`PosScreen.tsx` `TERMINAL`) | GW | Implement the route from the `terminal`/`merchant` tables; let the POS pick a terminal |
| POS-G12 | No PIN entry (MCN-305-AC4 deferred) | Plan Ruling R1; the gateway never forwards DE 52 (R-12) | GW + WEB | Close R-12, then restore the PIN pad and `encryptedPinBlock` |
| POS-G13 | Completion accepts any known RRN; the mock and the gateway disagree on an unknown one | `CreateCompletion` only `tranLog.Get`s the RRN; dev:mock returns 201 DECLINED RC `25`, the gateway 404 `unknown-transaction` | GW + WEB | Gateway: 409 `conflict` unless the original is an approved PREAUTH; align the mock with the gateway |
| POS-G14 | A retry after an HTTP failure creates a new Idempotency-Key | `pos-client.ts` calls `crypto.randomUUID()` inside each `mutationFn`; after a 500/502 whose outcome is unknown, pressing "Thanh toán" again can charge twice | WEB | Generate the key per draft and reuse it until the user changes the draft |
| POS-G17 | The issuer debited the account for every processing code, so a refund took money out of the customer's account | #114 review; `Authorize` always ran the purchase debit | ISS | **Fixed** in #119: a refund credits the customer (REFUND journal D `SETTLEMENT_SUSPENSE` / C customer, no funds or velocity check), and its reversal debits them back. Open: a refund reversal past the overdraft floor fails `chk_available_floor` and the SAF repeats it (decision pending) |
| POS-G18 | Reversals never restored account balances: `LocateAndReverse` posted the reversing journal but never updated `account.ledger_balance`/`available_balance`, so every reversed purchase left the customer debited while the ledger said the money came back. Live stack: `crd_second0006` read 169 446 500 against 170 253 500 from its postings, a gap of 807 000 = its three reversals (235 500 + 321 500 + 250 000); accounts without reversals matched. Everything that reads these columns was wrong after a reversal: the Card Admin balances (cards-page.md §4.2), the web Journey money panel, and the chaos ledger check (CHA-G1, being fixed in the gateway) | `LocateAndReverse` / `LedgerRepository.postReversal` had no `UPDATE account` | ISS | **Fixed** in #119: the reversal mirrors the original journal (directions swapped) and moves the balance through the same `AccountLockRepository.adjust` Authorize uses, in the transaction that marks the row `REVERSED`. Tests: every account ends at opening + Σ(credits − debits); a 200-ordering property test for purchases and refunds. Running stacks need a reseed, since no data fix is shipped |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version |
| 1.1 | 2026-09-25 | POS-G17 (refunds credit) and POS-G18 (reversals never restored balances) added and marked Fixed in #119; balance inquiry approved with DE 54 (POS-G6 note). |
