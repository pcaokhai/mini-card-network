# Cards and accounts page: API contract

| | |
| --- | --- |
| Document | `docs/api/cards-page.md` |
| Version | 1.2 |
| Status | Approved for integration |
| Date | 2026-09-25 |
| Screen | routes `/cards` and `/cards/{cardRef}`, container `web-next/src/app/(console)/cards/CardsScreen.tsx` |
| Stories | MCN-309 (WEB), MCN-308 (ISS, Issuer Admin API) |
| Provider(s) | issuer-jpos Admin API (`issuer-jpos/src/main/java/io/mcn/issuer/adapter/http/CardAdminController.java`) on :8081 |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/cards-client.ts` |
| Design reference | canvas `project/Cards.dc.html` (docs/01-prd.md §7.1); plan `docs/plans/MCN-309-cards-canvas.md` |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only). Write examples come from dev:mock (`web-next/src/mocks/pages/cards.ts`) and are labelled. |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and are not repeated.

## 1. Purpose and scope

The page lets an operator pick one of the issuer's cards and see its status, balances, holds, limits and double-entry ledger. From it they can block or unblock the card and change its daily and per-transaction limits. This contract covers the six `/v1/cards*` calls the page makes through the BFF.

Out of scope:
- `/v1/accounts` (listed in docs/04 §1, not built);
- `GET /v1/cards/{cardRef}/audit` (issuer since #115, CARDS-G2), which the page doesn't call yet;
- the POS tiles and the Journey money panel, which read the same card calls but have their own documents.

## 2. Page map

| UI region (canvas / i18n label) | Data shown | Call(s) | Refresh |
| --- | --- | --- | --- |
| Title "Thẻ và tài khoản" + subtitle | static | none | none |
| "Thẻ của khách hàng" list | holder, `•••• last4`, status badge per card, sorted by last four digits (Ruling R2) | §4.1 | on mount; after any block/unblock (invalidates `["cards"]`) |
| Card visual (brand "MCN Debit", `•••• •••• •••• last4`, holder, "Hết hạn {expiry}", lock overlay "Thẻ đang bị khóa") | `maskedPan` (last 4 only), `holderName`, `expiry`, derived status | §4.2 | with §4.2 |
| "Trạng thái thẻ" panel: badge, description, "Khóa thẻ" / "Mở khóa thẻ", inline confirm, audit line | `status`, `expiry`; session-local audit echo | §4.2, §4.4, §4.5 | with §4.2 |
| "Số dư": "Số dư trong sổ", "Đang tạm giữ", "Có thể chi tiêu", hold notes | `ledgerBalance`, `holds[]` (ACTIVE only), `availableBalance` | §4.2 | with §4.2 |
| "Hạn mức": two sliders, usage bar, "Đã dùng hôm nay …" | `limits`, `usedToday`, ETag | §4.2, §4.6 | with §4.2; after a limit save (invalidates `["cards", cardRef]`) |
| "Lịch sử tiền vào, tiền ra" (Easy) / "Bút toán kép (journal)" (Expert) | journal entries, 8 per page, "Xem thêm" / "Load more" | §4.3 | on mount; next page on click |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency (If-Match/ETag) | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/cards` | issuer :8081 | Card list | on mount; refetch on window focus (TanStack default); invalidated after block/unblock | n/a | none | Real |
| 4.2 | `GET /v1/cards/{cardRef}` | issuer :8081 | Card detail, balances, holds, limits, usage | on selecting a card; invalidated after any write | n/a | returns `ETag` (versions the limits, §4.2 rule 6) | Real |
| 4.3 | `GET /v1/cards/{cardRef}/ledger?limit=8&cursor=…` | issuer :8081 | Journal entries with postings, newest first | first page on mount; next page on "Xem thêm" / "Load more" | n/a | none | Real |
| 4.4 | `POST /v1/cards/{cardRef}/blocks` | issuer :8081 | Block the card | "Xác nhận khóa" | required, new UUID per click | none | Real |
| 4.5 | `DELETE /v1/cards/{cardRef}/blocks` | issuer :8081 | Unblock the card | "Xác nhận mở khóa" | required, new UUID per click | none | Real |
| 4.6 | `PUT /v1/cards/{cardRef}/limits` | issuer :8081 | Replace the ALL/DAILY and ALL/PER_TXN limits | slider released (pointer up or key up), only when the value changed | required, new UUID per save | `If-Match` required; 412 on mismatch | Real |
| – | WebSocket | – | none | – | – | – | – |
| – | Audit line under the status panel | client only | Echo of this session's own block/unblock | on mutation success | – | – | Client only (`GET …/audit` exists since #115; the page doesn't read it yet) |

The BFF routes every `/api/v1/cards*` request to `ISSUER_ADMIN_URL` (default `http://localhost:8081`). It forwards only these request headers: `accept`, `content-type`, `idempotency-key`, `if-match`, `traceparent`. It returns only these response headers: `content-type`, `etag`, `location`, `retry-after` (`route.ts:9-10`).

## 4. Calls

### 4.1 GET /v1/cards

**Summary.** Every card the issuer holds, masked.

**Request.** No parameters or body. The contract defines no filter or paging for this call.

**Response.** `200`, array of `CardSummary`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `cardRef` | string | ✓ | list link `href=/cards/{cardRef}`; `aria-current` on the selected one | not shown | not shown |
| `maskedPan` | string, first 6 + `*` + last 4 | ✓ | list line `•••• {last4}`; colour swatch `data-tag` (matched on the last 4) | last 4 only | last 4 only |
| `holderName` | string | ✓ | list holder | as sent | as sent |
| `status` | `CardStatus` | ✓ | status badge (with `expiry`, see rule 3) | "Đang hoạt động" / "Đã khóa" / "Đã hết hạn" | `ACTIVE`, `BLOCKED`, …, or `EXPIRED` when derived |
| `expiry` | string `MM/YY` | ✓ | badge derivation | – | – |

**Provider rules.**
1. The list is ordered by the provider's `CardRepository.findAll()`. The UI re-sorts by the last four digits of `maskedPan` (Ruling R2), so the provider needn't guarantee an order.
2. `maskedPan` = BIN (6) + `*` × (16 − 6 − 4) + last 4. The masking assumes a 16-digit PAN (`maskedPan()`, the `ponytail:` note).
3. `expiry` is `card.expiry_yymm` rendered as `MM/YY`. The UI treats a card as expired from the first day of the month after its expiry month, whatever `status` says (`cards-model.ts` `isPastExpiry`, Ruling R3). Since #115 the issuer derives the same thing on read (CARDS-G4): an `ACTIVE` or `BLOCKED` card past its expiry month reads `EXPIRED` (the row isn't changed); `LOST`, `STOLEN` and `PIN_BLOCKED` outrank expiry.
4. Status mapping for the UI: `ACTIVE` → active; any other value that isn't expired (`BLOCKED`, `LOST`, `STOLEN`, `PIN_BLOCKED`) → locked.

**Errors.** The handler emits no problem. An unexpected failure is Javalin's default `500` with the plain-text body `Server Error`. Through the BFF, an unreachable issuer becomes `502 upstream-unavailable`.

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 502 | `upstream-unavailable` (BFF) | issuer not reachable | alert "Không tải được thẻ này. Thử tải lại trang."; no list, no detail |
| 500 | none (plain text) | unhandled exception | same alert |

**Example** (real, local stack 2026-09-25, trimmed):
```http
GET /api/v1/cards
200 OK
[{"cardRef":"crd_normal0001","maskedPan":"970436******4417","holderName":"Nguyen Minh Anh","status":"ACTIVE","expiry":"11/28"},
 {"cardRef":"crd_blockd0003","maskedPan":"970436******3310","holderName":"Le Quoc Bao","status":"BLOCKED","expiry":"07/27"},
 {"cardRef":"crd_expird0004","maskedPan":"970436******7765","holderName":"Pham Gia Huy","status":"ACTIVE","expiry":"08/26"}, …]
```

**Notes.** Query key `["cards"]`. TanStack defaults (`QueryProvider.tsx` sets none): `staleTime` 0, 3 retries with back-off, refetch on window focus. No cache headers. Rate limit: not enforced.

### 4.2 GET /v1/cards/{cardRef}

**Summary.** One card with balances, holds, limits and today's usage, plus an `ETag` for the limits.

**Request.**

| name | in | type | required | constraints | default |
| --- | --- | --- | --- | --- | --- |
| `cardRef` | path | string | ✓ | contract pattern `^crd_[A-Za-z0-9]{10,32}$`; any other value → `400 validation-error` (CARDS-G13) | – |

**Response.** `200`, `CardDetail` (= `CardSummary` + the fields below), header `ETag`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `status`, `expiry` | see §4.1 | ✓ | "Trạng thái thẻ" badge + description | "Thẻ dùng được bình thường ở mọi máy POS." / "Mọi giao dịch mới sẽ bị từ chối với lý do thẻ bị khóa." / "Thẻ đã quá ngày hết hạn. Khách cần được phát hành thẻ mới." | `card.status = {status} · …` / `… → participant Validate trả RC 62.` / `expiry {expiry} < business date → RC 54.` |
| `ledgerBalance` | `Money` | ✓ | "Số dư trong sổ" | formatted ₫ | label `ledger_balance` |
| `holds[]` | array | ✓ | "Đang tạm giữ" total (ACTIVE only) and one note per ACTIVE hold | "Tạm giữ cho {merchant} · {amount}", "Tự giải phóng {dd/MM}" | "auth_hold ACTIVE · {merchant} · {amount}" |
| `availableBalance` | `Money` | ✓ | "Có thể chi tiêu" | formatted ₫ | label `available_balance` |
| `limits.dailyAmount` | `Money` | ✓ | slider "Hạn mức mỗi ngày" (1 000 000–50 000 000 ₫, step 1 000 000) | value, or "Chưa đặt" when 0 | label "card_limit ALL / DAILY" |
| `limits.perTransactionAmount` | `Money` | ✓ | slider "Hạn mức mỗi giao dịch" (500 000–20 000 000 ₫, step 500 000) | value, or "Chưa đặt" when 0 | label "card_limit ALL / PER_TXN" |
| `limits.dailyCount` | integer \| null | ✓ (nullable) | not shown; echoed unchanged in §4.6 | – | – |
| `usedToday` | `Money` | ✓ | usage bar `scaleX(used / daily)` and "Đã dùng hôm nay {used} · {percent}% hạn mức" ("Đã dùng hôm nay {used}" when the daily limit is 0) | same | same |
| header `ETag` | string | ✓ | kept with the query data; sent as `If-Match` by §4.6 | – | – |

**Provider rules.**
1. `ledgerBalance` and `availableBalance` are read straight from the `account` row's `ledger_balance` and `available_balance` columns (`AccountRepository.findById`) in the account currency (`704`), not summed from the ledger. Those columns change only through `AccountLockRepository.adjust`, in the same transaction as the journal that explains the change, so balance = opening + Σ credits − Σ debits of the account's postings. Before #119 a reversal wrote its journal but never updated the columns, so any card with a reversal showed a balance too low by the reversed amounts (pos-page.md §9 POS-G18).
2. `holds` is always `[]`, because no PREAUTH flow ships yet (MCN-603; the `ponytail:` note in `cardDetail()`). The dev:mock card 4417 has one hold, as in the canvas (Ruling R9).
3. `limits` holds the `card_limit` rows with `txn_type = 'ALL'`. A missing row reads as `amount: 0`, and the UI shows "Chưa đặt" for it (Ruling R6).
4. `usedToday` = `SUM(velocity_counter.txn_amount)` for the card, with `period = 'DAILY'` and `period_key` = `system_state.current_business_date` (CARDS-G12). While no cutover has written that row, it falls back to the calendar date, which is also what the ISO path keys the counters with.
5. Only masked card data is returned: no PAN, CVV, track data, PIN data or key material.
6. **ETag.** `"<limits version>-<representation hash>"`. The limits version is the hex SHA-256 over `PERIOD:maxAmount:maxCount;` for each limit row sorted by period; the representation hash is the first 16 hex of the SHA-256 of the response body. **The ETag versions the limits:** If-Match (§4.6) compares only the first part, so a status or balance change doesn't fail a limits save. The second part makes the whole value change with the body (CARDS-G1).
7. **Caching.** Every `/v1/cards*` response is `Cache-Control: no-store`. The issuer still answers `304` to a matching `If-None-Match`, but because the ETag covers the whole body, a 304 now means nothing changed; a changed balance or status gets `200` (CARDS-G1). The BFF doesn't forward `If-None-Match` (Release R5.2).

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 404 | `not-found` | no card with that `cardRef` | detail replaced by "Không tải được thẻ này. Thử tải lại trang." |
| 502 | `upstream-unavailable` (BFF) | issuer not reachable | same |

**Example** (real, local stack 2026-09-25):
```http
GET /api/v1/cards/crd_normal0001
200 OK
ETag: "c90d52a3f072ce9bc397372bdcbb6ec785c513b4652a4b08df1a8efd5bd4260d"
{"cardRef":"crd_normal0001","maskedPan":"970436******4417","holderName":"Nguyen Minh Anh","status":"ACTIVE","expiry":"11/28",
 "ledgerBalance":{"amount":2982000,"currency":"704"},"availableBalance":{"amount":2982000,"currency":"704"},"holds":[],
 "limits":{"dailyAmount":{"amount":8000000,"currency":"704"},"perTransactionAmount":{"amount":9500000,"currency":"704"},"dailyCount":null},
 "usedToday":{"amount":2018000,"currency":"704"}}
```
```http
GET /api/v1/cards/crd_nope0000000
404
{"type":"https://mcn.local/problems/not-found","title":"card not found","status":404,"detail":"card not found: crd_nope0000000","instance":"/v1/cards/crd_nope0000000","traceId":"bb9bb3e6e18f4837a32ea9f49d976751"}
```

**Notes.** Query key `["cards", cardRef]`. `useCard` returns `{ card, etag }`, with `etag` read from the raw `Response`. TanStack defaults apply. The provider sends `Cache-Control: no-store`. Rate limit: not enforced.

### 4.3 GET /v1/cards/{cardRef}/ledger

**Summary.** The card account's journal entries, newest first, each with all its postings, paged by cursor.

**Request.**

| name | in | type | required | constraints | default |
| --- | --- | --- | --- | --- | --- |
| `cardRef` | path | string | ✓ | as §4.2 | – |
| `limit` | query | integer | – | contract 1–200; the UI sends `8` (`LEDGER_PAGE_SIZE`) | 50 |
| `cursor` | query | string | – | **the `journalId` of the last entry on the previous page** (a decimal integer as a string) | none (first page) |

**Response.** `200`, `{ items: JournalEntry[], nextCursor: string | null }`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `items[].journalId` | string (decimal) | ✓ | row key; the next cursor | not shown | not shown |
| `items[].occurredAt` | RFC 3339 UTC | ✓ | "Giờ" column, `HH:mm` in the browser's local time zone | same | same |
| `items[].description` | string | ✓ | "Nội dung" | as sent, unless it's the generated `<TYPE> journal <id>`, which shows the entry type name ("Mua hàng", "Hoàn tiền tự động", …) instead (Ruling R7) | same |
| `items[].entryType` | enum | ✓ | fallback description | label from `cards.ledger.type` | same |
| `items[].rrn` | string \| null | ✓ (nullable) | not shown (the Journey money panel uses it) | – | – |
| `items[].postings[]` | array | ✓ | column a and column b | "Loại": "Tiền ra" / "Tiền vào" / "Không đổi"; "Số tiền": signed net effect on `ACC-*` postings (`−250.000 ₫`) | "Vế Nợ": `Nợ {debit account}`; "Vế Có": `Có {credit account} · {amount}`, or "Không hạch toán" when there are no postings |
| `nextCursor` | string \| null | ✓ | shows "Xem thêm" / "Load more" while non-null | – | – |

**Provider rules.**
1. **Ordering.** Entries are journal IDs that have at least one posting on the card's account, in `journal_id DESC` order. Postings within an entry are in `ledger_posting.id` order.
2. **Paging.** `cursor` is exclusive: the page holds journals with `journal_id < cursor`. `nextCursor` = the last item's `journalId` when the page came back full (`items.length == limit`), else `null`. When the total is an exact multiple of `limit`, one extra request returns `{"items":[],"nextCursor":null}`.
3. **Account naming.** A customer account posting carries `account = "ACC-<cardRef>"` (`account.account_no`, set by `SeedLoader`). A GL posting carries the GL code (for example `SETTLEMENT_SUSPENSE`). The UI counts only `ACC-` postings towards the Easy "Số tiền" effect (`cards-model.ts` `isCustomer`). The contract's description ("customer account number (masked)") doesn't match this; the value isn't card data (§9 CARDS-G14).
4. **Balance.** Every entry balances: Σ DEBIT = Σ CREDIT per journal, enforced at COMMIT by the deferred constraint trigger `trg_journal_balanced` (`issuer-jpos/src/main/resources/db/migration/V2__issuer_baseline.sql`; docs/05 §2).
5. `description` is always generated as `<entryType> journal <journalId>` (for example `PURCHASE journal 244`). Human-readable descriptions appear only in dev:mock.
6. On the live stack only `PURCHASE` and `REVERSAL` journals exist (`LedgerRepository.postPurchase` / `postReversal`). Declined transactions post nothing, so they don't appear (Ruling R8).
7. `amount` is integer minor units; `currency` is ISO 4217 numeric.

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 404 | `not-found` | unknown `cardRef` | the ledger panel shows "Chưa có bút toán nào" (the detail call fails first and replaces the panel) |
| 400 | `validation-error` (`errors[]` on `limit` / `cursor`) | `limit` outside 1–200, or `cursor` not a positive integer (CARDS-G8) | same empty state (the UI never sends these values) |

**Example** (real, local stack 2026-09-25, `limit=2`):
```http
GET /api/v1/cards/crd_normal0001/ledger?limit=2
200 OK
{"items":[
  {"journalId":"244","occurredAt":"2026-09-25T07:58:03.676262Z","description":"PURCHASE journal 244","entryType":"PURCHASE","rrn":"626807000352",
   "postings":[{"account":"ACC-crd_normal0001","direction":"DEBIT","amount":{"amount":10000,"currency":"704"}},
               {"account":"SETTLEMENT_SUSPENSE","direction":"CREDIT","amount":{"amount":10000,"currency":"704"}}]},
  {"journalId":"243", …}],
 "nextCursor":"243"}
```
Next page: `GET …/ledger?limit=2&cursor=243` → journals `242`, `236`, `nextCursor` `"236"`.

**Notes.** `useCardLedgerPages(cardRef, 8)` (`useInfiniteQuery`, key `["cards", cardRef, "ledger", "pages", 8]`, `getNextPageParam` = `nextCursor`). A block or limit change doesn't invalidate the ledger. Rate limit: not enforced.

### 4.4 POST /v1/cards/{cardRef}/blocks

**Summary.** Moves an `ACTIVE` card to `BLOCKED` and writes one `audit_log` row in the same transaction.

**Request.**

| name | in | type | required | constraints | notes |
| --- | --- | --- | --- | --- | --- |
| `cardRef` | path | string | ✓ | as §4.2 | |
| `Idempotency-Key` | header | UUID | ✓ | a UUID (`8-4-4-4-12` hex), else `400 insufficient-idempotency-key` | new `crypto.randomUUID()` per confirm |
| `Content-Type` | header | string | ✓ | `application/json` | |
| `traceparent` | header | string | – | forwarded by the BFF | not read by the handler |

Body:

| field | type | required | constraints | notes |
| --- | --- | --- | --- | --- |
| `reason` | enum `CUSTOMER_REQUEST`, `LOST`, `STOLEN`, `FRAUD_SUSPECTED` | ✓ | anything else → `400 validation-error` (CARDS-G5) | the UI always sends `CUSTOMER_REQUEST` (Ruling R4) |

**Response.** `200`, `CardDetail` with the new status: `LOST` for `LOST`, `STOLEN` for `STOLEN`, `BLOCKED` for `FRAUD_SUSPECTED` and `CUSTOMER_REQUEST`. The fields are as in §4.2; the response has no `ETag` header.

**Provider rules.**
1. **Order of checks.** (a) `cardRef` pattern; (b) `Idempotency-Key` present and a UUID; (c) replay lookup on (key, `"POST /v1/cards/{cardRef}/blocks"`); (d) `reason` in the enum; (e) card exists and, under `SELECT … FOR UPDATE` on the card, its status as read (§4.2 rule 3) is `ACTIVE`.
2. **Replay.** Same key, same route, same body hash → the stored status and body, byte for byte. Same key, different body → `422`. Records expire after 24 h (docs/04 §2): an older record is ignored, so the key counts as new, and each admin write purges expired records (CARDS-G10). **Requests in flight together with the same key** don't race: every write takes the card's row lock, and the first request commits its idempotency record before releasing it. Each later request re-reads the record once it holds the lock and replays it (the same `200` body, or `422` for a different body). So none of them gets a `500` from the record's primary key, and the change and its audit row happen once.
3. The status update, the `audit_log` row (`action = CARD_BLOCKED`, `entity_type = card`, `entity_id = cardRef`, before `{"status":"ACTIVE"}`, after `{"status":"<new status>","reason":"<reason>"}`) and the idempotency record commit in one transaction.
4. `audit_log.actor` = the `X-Actor` header, or `"unknown"`. The BFF neither sends nor forwards `X-Actor`, so every audit row from the page says `unknown` (§9 CARDS-G3).
5. Effect on authorization: the issuer's Validate participant declines the next 0100/0200 for this card with RC `62` (the Expert description says so).

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 400 | `insufficient-idempotency-key` | header missing or not a UUID | "Không đổi được trạng thái thẻ: {detail}" under the panel |
| 404 | `not-found` | unknown card | same, with the detail |
| 400 | `validation-error` | `reason` missing or outside the enum (`errors[0].field = reason`), or a malformed `cardRef` | same |
| 409 | `conflict` | card isn't `ACTIVE`, including an expired card (detail: `card {cardRef} is {status}, cannot block it`) | same |
| 422 | `idempotency-key-mismatch` | key reused with a different body | same |
| 502 | `upstream-unavailable` (BFF) | issuer not reachable | same |

**Example** (dev:mock, `web-next/src/mocks/pages/cards.ts`, trimmed). Not run on the real stack by rule.
```http
POST /api/v1/cards/crd_normal0001/blocks
Idempotency-Key: 00000000-0000-4000-8000-000000000001
Content-Type: application/json

{"reason":"CUSTOMER_REQUEST"}

200 OK
{"cardRef":"crd_normal0001","maskedPan":"970436******4417","holderName":"Nguyễn Minh Anh","status":"BLOCKED","expiry":"11/28",
 "ledgerBalance":{"amount":5000000,"currency":"704"},"availableBalance":{"amount":3500000,"currency":"704"}, …}
```

**Notes.** On success the UI invalidates every `["cards"]` query (the list badge and the detail) and appends an audit line: Easy "Đã ghi nhật ký: khóa thẻ •••• {last4} lúc {HH:mm}", Expert "audit_log · CARD_BLOCKED · card {last4} · actor ops". The mutation has no retry. The dev:mock handler ignores `Idempotency-Key` and never returns 409.

### 4.5 DELETE /v1/cards/{cardRef}/blocks

**Summary.** Moves a `BLOCKED` card back to `ACTIVE`, audited.

**Request.** The same path and headers as §4.4, with no body.

**Response.** `200`, `CardDetail` with `status: "ACTIVE"`.

**Provider rules.** The same as §4.4 with these differences: the precondition is `status == BLOCKED`, the action is `CARD_UNBLOCKED`, and the replay route is `"DELETE /v1/cards/{cardRef}/blocks"`. Only a card that reads `BLOCKED` can be unblocked. `LOST`, `STOLEN`, `PIN_BLOCKED` and `EXPIRED` get `409 conflict` (CARDS-G5). The UI still offers "Mở khóa thẻ" for every locked kind; hiding it is the WEB half of CARDS-G5.

**Errors.** As §4.4, where 409 means the card isn't `BLOCKED` (detail `card {cardRef} is {status}, cannot unblock it`).

**Example** (dev:mock): `DELETE /api/v1/cards/crd_blockd0003/blocks` with `Idempotency-Key` → `200` `{"cardRef":"crd_blockd0003", …, "status":"ACTIVE", …}`.

**Notes.** As §4.4. The audit line reads "mở khóa" / `CARD_UNBLOCKED`.

### 4.6 PUT /v1/cards/{cardRef}/limits

**Summary.** Replaces the card's `ALL` DAILY and PER_TXN limits under optimistic concurrency.

**Request.**

| name | in | type | required | constraints | notes |
| --- | --- | --- | --- | --- | --- |
| `cardRef` | path | string | ✓ | as §4.2 | |
| `Idempotency-Key` | header | UUID | ✓ | a UUID (`8-4-4-4-12` hex), else `400 insufficient-idempotency-key` | new per save |
| `If-Match` | header | string | ✓ | its limits version (the part before `-`, §4.2 rule 6) must equal the current one; a limits-only ETag from before #115 is also accepted. Only one exact ETag is honoured: `*` and a comma-separated list are compared as a single literal and fail. A missing `If-Match` gets `412 precondition-failed`, not RFC 6585's 428, because the catalogue has no 428 | the UI sends the ETag from its last §4.2 read, or `""` when it had none |
| `Content-Type` | header | string | ✓ | `application/json` | |

Body (`CardLimits`):

| field | type | required | constraints | notes |
| --- | --- | --- | --- | --- |
| `dailyAmount.amount` | int64 | ✓ | integer > 0 (`card_limit` CHECK) | the UI's slider range is 1 000 000–50 000 000 |
| `dailyAmount.currency` | string | ✓ | must equal the card's currency | the UI echoes the card's currency |
| `perTransactionAmount.amount` | int64 | ✓ | integer > 0 and ≤ `dailyAmount.amount` | UI range 500 000–20 000 000 |
| `perTransactionAmount.currency` | string | ✓ | must equal the card's currency | |
| `dailyCount` | integer \| null | – | null or an integer > 0 | the UI echoes the current value unchanged |

**Response.** `200`, `CardDetail` with the new `limits`, plus a new `ETag` header.

**Provider rules.**
1. **Order of checks.** (a) `cardRef` pattern; (b) `Idempotency-Key` present and a UUID; (c) replay lookup, so a retried successful save gets its stored `200` even though its If-Match is now stale; (d) card exists; (e) body validation (§4.6 body table), every failure listed in `errors[]`; (f) inside the write transaction, under `SELECT … FOR UPDATE` on the card, the If-Match limits version equals the current one.
2. **ETag.** As §4.2 rule 6. A save that changes only `dailyCount` still changes the limits version. A block or unblock doesn't.
3. The upsert of both `card_limit` rows (`txn_type = ALL`, periods `PER_TXN` and `DAILY`, with `max_count = dailyCount` on DAILY), the `audit_log` row (`CARD_LIMITS_UPDATED`, before/after limit maps) and the idempotency record commit in one transaction.
4. Of two concurrent saves carrying the same ETag, exactly one succeeds; the other reads the winner's limits under the lock and gets `412` (CARDS-G6).
5. `perTransactionAmount ≤ dailyAmount` is enforced for new saves (CARDS-G7). Rows saved before #115 can still break it (`crd_normal0001` on the local stack: 9 500 000 per transaction against 8 000 000 per day) until they are saved again.
6. **Ceilings are positive.** Amounts must be > 0 and `dailyCount` null or > 0, the same rule as the `card_limit` CHECKs (`max_amount > 0`, `max_count > 0`). A zero ceiling gets `400 validation-error` with the field in `errors[]`, never a 500 from the database. To stop spending, block the card (§4.4). Decided by the lead on #115 (CARDS-G7).
7. **Same-key requests in flight** replay the first one's stored response under the card lock, as in §4.4 rule 2.

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 400 | `insufficient-idempotency-key` | header missing or not a UUID | "Không lưu được hạn mức: {detail}" |
| 400 | `validation-error` | the body isn't a JSON object, or fields fail the body table; `errors[]` has one `{field, message}` per failure (for example `dailyAmount.amount`, `perTransactionAmount.currency`, `dailyCount`) | same |
| 404 | `not-found` | unknown card | same |
| 412 | `precondition-failed` | `If-Match` missing or stale | banner "Someone changed this card. Reload to continue." with "Tải lại" (MCN-309-AC3); "Tải lại" drops the drafts and refetches §4.2 |
| 422 | `idempotency-key-mismatch` | key reused with a different body (checked before If-Match) | "Không lưu được hạn mức: {detail}" |

**Example** (dev:mock; the mock ETag is a version counter `"v1"`, not a hash):
```http
PUT /api/v1/cards/crd_normal0001/limits
Idempotency-Key: 00000000-0000-4000-8000-000000000002
If-Match: "v1"
Content-Type: application/json

{"dailyAmount":{"amount":12000000,"currency":"704"},"perTransactionAmount":{"amount":5000000,"currency":"704"},"dailyCount":null}

200 OK
ETag: "v2"
{"cardRef":"crd_normal0001", …, "limits":{"dailyAmount":{"amount":12000000,"currency":"704"},"perTransactionAmount":{"amount":5000000,"currency":"704"},"dailyCount":null}, …}
```
Stale (dev:mock and real, same body):
```json
{"type":"https://mcn.local/problems/precondition-failed","title":"ETag mismatch","status":412,"detail":"If-Match does not match the current ETag."}
```

**Notes.** Sliders save on release, not on each step: each save carries the ETag, so a burst would 412 against its own first write (`LimitsPanel.tsx`). On success the UI invalidates `["cards", cardRef]`. The client maps status 412 to `PreconditionFailedError` before looking at the body. The mutation has no retry.

## 5. Real-time events

None. The page consumes no WebSocket events. Balances change only on refetch (mount, window focus or invalidation).

## 6. Security and compliance

| Topic | Rule on this page |
| --- | --- |
| PAN | Only `maskedPan` (first 6 + last 4) crosses the API. The markup renders the **last 4 only** (`CardVisual.tsx`, `CardList.tsx`). No full PAN, CVV, track data, PIN or PIN block is ever returned. |
| Identifiers | Cards are addressed by `cardRef`, never by PAN. `ACC-<cardRef>` account numbers carry no card data. |
| Key material | None on this page. |
| Audit | Block, unblock and limit changes each write one `audit_log` row (before/after JSON) in the same transaction as the change (MCN-308-AC2). `audit_log` is append-only (docs/05 §2). The actor is `unknown` today (CARDS-G3). The rows can be read through `GET /v1/cards/{cardRef}/audit` (#115); the UI's audit line is still a session echo (Ruling R5). |
| Destructive actions | Block and unblock need an inline confirmation ("Khóa thẻ •••• {last4}? Mọi giao dịch mới sẽ bị từ chối cho tới khi mở khóa." / "Mở khóa thẻ •••• {last4}? Thẻ sẽ dùng lại được ngay."), and the confirm button is disabled while pending. Expired cards get no toggle. |
| Idempotency | Every write carries a fresh UUID `Idempotency-Key` per user action (docs/04 §2). |

## 7. Non-functional requirements

No NFR in docs/02 §2 sets a latency for the Admin API. The figures below are **observed** on the local stack through the BFF, 60 sequential requests each, on 2026-09-25.

| Call | p50 | max of 60 (≈ p99) | Payload |
| --- | --- | --- | --- |
| 4.1 list | 5.3 ms | 7.9 ms | ≈ 0.7 KB for 6 cards |
| 4.2 detail | 5.8 ms | 22.5 ms | ≈ 0.4 KB |
| 4.3 ledger, `limit=8` | 6.0 ms | 11.9 ms | ≈ 2.8 KB per page (≈ 9.5 KB at `limit=200`) |
| 4.4–4.6 writes | not measured (no writes on the shared stack) | – | ≈ 0.4 KB |

- Polling: none. Load is one list call plus one detail call plus one ledger page per card view, and more on window focus.
- Pagination: the ledger uses 8 per page from the UI. The provider enforces the contract range 1–200: `limit=1000` gets `400 validation-error` (CARDS-G8).

## 8. UI states

| State | Driver | What renders |
| --- | --- | --- |
| Loading | 4.1 or 4.2 pending | heading only; the detail column renders nothing until 4.2 resolves |
| Empty list | 4.1 returns `[]` | "Thẻ của khách hàng" with "Chưa có thẻ nào"; no detail |
| Empty ledger | 4.3 returns no items | the table header plus "Chưa có bút toán nào" |
| Partial | 4.3 fails while 4.2 succeeds | card, balance and limits render; the ledger shows "Chưa có bút toán nào" (the error isn't surfaced) |
| Error | 4.1 fails | alert "Không tải được thẻ này. Thử tải lại trang." and no list |
| Error | 4.2 fails (404, 5xx, 502) | the detail column is replaced by the same alert; the list still renders |
| Write failed | 4.4 / 4.5 / 4.6 | inline alerts in §4.4 and §4.6; a 412 shows the reload banner |
| Provider not available | issuer down | the BFF returns 502 `upstream-unavailable`, which renders as the error states above |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| CARDS-G1 | The ETag covers the limits only, so every card without limits shares `"e3b0c442…b855"`, and the issuer answers 304 to a matching `If-None-Match`. A direct caller can get a stale 304 after a balance or status change. The BFF is safe only because it drops `If-None-Match`. | `CardAdminController.java:350-363`; 4 of 6 local cards return the same ETag; direct `If-None-Match` → `304` | ISS | **Fixed** in #115: Every `/v1/cards*` response sends `Cache-Control: no-store`; the ETag is `"<limits version>-<representation hash>"`, so If-None-Match never 304s a changed card and If-Match still versions the limits (§4.2 rules 6–7) |
| CARDS-G2 | No audit read API; the UI echoes this session's actions only | `CardDetail.tsx:24`; R5.3 "Still open" | ISS | **Fixed** in #115: `GET /v1/cards/{cardRef}/audit`, newest first, id cursor, actor/before/after from `audit_log`. Consuming it in the UI is WEB work |
| CARDS-G3 | `audit_log.actor` is always `unknown`: the BFF never sets `X-Actor` and doesn't forward it. The Expert audit line claims "actor ops". | `route.ts:9`; `CardAdminController.java:365-368`; docs/04 §2 "Actor" | WEB | BFF sets `X-Actor` (and adds it to the forwarded headers) |
| CARDS-G4 | Expiry isn't applied by the issuer: `crd_expird0004` (`08/26`) reads `ACTIVE` on 2026-09-25. The UI derives "Đã hết hạn" (Ruling R3). | live `GET /v1/cards` | ISS | **Fixed** in #115: `EXPIRED` derived on read (domain `CardLifecycle`); the row isn't changed |
| CARDS-G5 | `reason` isn't validated or persisted. A block always sets `BLOCKED`, so `LOST`, `STOLEN` and `FRAUD_SUSPECTED` are lost. The UI offers "Mở khóa thẻ" for `LOST`, `STOLEN` and `PIN_BLOCKED`, which the provider refuses with 409. | `CardAdminController.java:90-121`; `cards-model.ts` `statusKind` | ISS + WEB | **Fixed** in #115: Issuer side: the reason is validated (400), mapped to a status, and stored in the audit `after`; unblocking anything but `BLOCKED` gets 409. The WEB half (hide unblock) is still open |
| CARDS-G6 | PUT limits checks If-Match before the idempotency replay, so a retried successful save gets 412. The If-Match check isn't atomic with the upsert (no row lock or version column), so two concurrent PUTs with the same ETag can both succeed. | `CardAdminController.java:171-199` | ISS | **Fixed** in #115: Replay before If-Match; If-Match and the upsert run under `SELECT … FOR UPDATE` on the card; a concurrency test proves exactly one of N same-ETag saves wins |
| CARDS-G7 | Limit validation is thin: no `errors[]`, `currency` ignored, `dailyCount` unchecked, and `perTransactionAmount > dailyAmount` accepted | `CardAdminController.java:185-194`; live `crd_normal0001` 9 500 000 / 8 000 000 | ISS | **Fixed** in #115: `errors[]` per field: positive amounts (`card_limit` CHECK), the card's currency, `dailyCount` null or > 0, `perTransaction ≤ daily` |
| CARDS-G8 | Ledger query parameters aren't validated: a non-integer `limit` or `cursor`, or `limit=0`, gives a plain-text `500 Server Error`, and there's no cap at 200 | live `?limit=abc`, `?limit=0`, `?cursor=abc` → 500; `?limit=1000` → 200 | ISS | **Fixed** in #115: `limit` 1–200 and a positive integer `cursor` on ledger and audit, else 400 `validation-error` |
| CARDS-G9 | Issuer problems use `Content-Type: application/json`, not `application/problem+json`. `traceId` is a random UUID, not the request's trace. No `X-Trace-Id` header. | live 404 headers; `CardAdminController.java:392-401` | ISS | **Fixed** in #115: One mapper (`ApiProblems`): `application/problem+json`, full-URI `type`, `instance`, `traceId` from `traceparent`, `X-Trace-Id` on every response, 500 `internal` for unexpected errors. The path in `instance`, `detail` and the 500 log line goes through the shared `PanMasker`. Javalin's own client errors keep their status (404 `not-found`, other 4xx `validation-error`) instead of becoming 500s, and an unmatched route gets a `not-found` problem instead of Javalin's plain text that echoes the path |
| CARDS-G10 | Idempotency records never expire (docs/04 §2: 24 h) | `IdempotencyRepository` has no TTL column or filter | ISS | **Fixed** in #115: Records older than 24 h are ignored on read and purged on each admin write |
| CARDS-G11 | `holds` is always `[]` (no PREAUTH, MCN-603). Journal descriptions are always generated, so real rows show only the type name. | `CardAdminController.java:283-284, 316` | ISS | MCN-603; description from merchant name |
| CARDS-G12 | `usedToday` keys on the issuer JVM's calendar date, not the ISO business date | `CardLimitRepository.amountToday` | ISS | **Fixed** in #115: Keyed by `system_state.current_business_date`, falling back to the calendar date while no cutover has written it. Consistent with ADR-007 (today = the business date, rolled at cutover). **When MCN-702 starts writing `system_state`, the ISO path (`ParseAndValidate`, which sets the business date that keys `velocity_counter`, plus the velocity rules) must switch to it in the same change;** otherwise `usedToday` and the velocity checks read different days. |
| CARDS-G13 | The provider doesn't enforce the `cardRef` pattern (any string → 404) | `CardAdminController.getCard` | ISS | **Fixed** in #115: 400 `validation-error` on a pattern mismatch |
| CARDS-G14 | The contract says the posting `account` is a "customer account number (masked)". The provider sends `ACC-<cardRef>`, which the UI relies on (Ruling R7). | `openapi.yaml` `JournalEntry`; `LedgerRepository.postingsForJournals` | PLAT (contracts) | **Fixed** in #115: No provider change needed: it already sends `ACC-<cardRef>`, which the reworded contract (#111) describes |
| CARDS-G15 | Two strings in `vi.json` are English: `cards.limits.stale` ("Someone changed this card. Reload to continue.", the MCN-309-AC3 wording) and `cards.ledger.expert.more` ("Load more") | `web-next/messages/vi.json` | WEB | Translate, keeping AC3's meaning |
| CARDS-G16 | An `Idempotency-Key` that isn't a UUID is accepted (docs/04 §2–3 say 400 `insufficient-idempotency-key`) | `CardAdminController` checked non-blank only | ISS | **Fixed** in #115: 400 `insufficient-idempotency-key` unless the key is a UUID |
| CARDS-G17 | Requests in flight together with the same key: the second one misses the replay lookup and gets 409 (block) or 412 (limits), or a 500 on the idempotency primary key without a lock | `replayIfPresent` read before the write transaction | ISS | **Fixed** in #115: the record is re-read under the card's row lock and replayed (§4.4 rule 2) |
| CARDS-G18 | The authorization path doesn't use `CardLifecycle`: `CheckCard.isNonActive` declines `BLOCKED`, `LOST` and `STOLEN` with RC 62 but not `PIN_BLOCKED`, which the Admin API reads as a locked card | `CheckCard.isNonActive` | ISS | **Fixed** in #119: `CheckCard` decides by `CardLifecycle.effectiveStatus` on the business date `ParseAndValidate` sets: RC 54 expired (from the first day after the expiry month, as this page reads it), RC 75 `PIN_BLOCKED` (docs/03 §8), RC 62 `BLOCKED`/`LOST`/`STOLEN`. A `BLOCKED` card past expiry now gets 54 |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version, verified against main @ `8d27c72` and the local stack (GET only). |
| 1.1 | 2026-09-25 | Issuer gap fixes in #115: CARDS-G1, G2, G4, G5 (issuer half), G6, G7, G8, G9, G10, G12 and G13 are marked Fixed, and G14 needed no provider change. §4 provider rules, request tables and error tables now describe the new behaviour. The `Idempotency-Key` must be a UUID, requests with the same key that are in flight together replay under the card lock (§4.4 rule 2), and limits must be positive (§4.6 rule 6). The G12 note covers MCN-702. Adds CARDS-G16 and G17 (Fixed) and G18 (open), and documents the If-Match forms (§4.6) and the PAN masking in problems (G9). |
| 1.2 | 2026-09-25 | CARDS-G18 marked Fixed in #119: authorization and the Admin API read card status through the same rule. §4.2 rule 1 states where the balances come from and the pre-#119 reversal gap. |
