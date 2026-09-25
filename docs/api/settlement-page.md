# Settlement and reconciliation page: API contract

| | |
| --- | --- |
| Document | `docs/api/settlement-page.md` |
| Version | 1.0 |
| Status | **Draft: provider not implemented** |
| Date | 2026-09-25 |
| Screen | route `/settlement`, container `web-next/src/app/(console)/settlement/SettlementScreen.tsx` |
| Stories | MCN-705 (WEB, Sprint 8); MCN-702 (GW + ISS, cutover and 0500), MCN-703 (SET, ingestion), MCN-704 (SET, reconciliation, breaks and clearing file). The providers are Sprint 9 and not built. |
| Provider(s) | settlement service (`settlement/`, :8082): planned. Today no service serves `/v1/settlement*`. |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/settlement-client.ts` |
| Design reference | canvas `project/Settlement.dc.html` (docs/01-prd.md §7.1); plan `docs/plans/MCN-705.md` ("Canvas pass" rulings R1–R14) |
| Verified against | main @ `8d27c72`: `contracts/openapi.yaml` and the dev:mock handlers `web-next/src/mocks/pages/settlement.ts` (`createSettlementHandlers`). On the local stack (2026-09-25, GET only) every call answers 404. |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and are not repeated.

Because no provider exists, "Provider rules" below are the rules the contract, docs/04 and the MCN-702/704 acceptance criteria set, and that the dev:mock handlers implement. A future provider must satisfy them, and the UI already relies on them. Where dev:mock simplifies a rule, §9 records it.

## 1. Purpose and scope

The page walks an operator through closing one business day:
1. cutover;
2. the 0500 totals exchange;
3. transaction-level reconciliation, with a break list to resolve;
4. clearing-file generation.

It also shows the net position the issuer owes the acquirer. This contract covers the six `/v1/settlement*` calls and the `SettlementDay`, `SettlementStage`, `TotalsRow`, `ReconBreak` and `ClearingFile` schemas.

Out of scope:
- the Kafka topics and `clearing_record` ingestion (MCN-703), which have no REST surface;
- the ISO 0800/201 and 0500/0510 messages themselves (docs/03 §7.6, §7.7);
- a days list or history, which the contract doesn't define (§9 SET-G7).

## 2. Page map

| UI region (canvas / i18n label) | Data shown | Call(s) | Refresh |
| --- | --- | --- | --- |
| Title "Chốt ngày và đối soát" + subtitle; primary button ("Chạy bước 1: khóa sổ", "Đang gửi tổng kết…", "Chạy bước 3: đối soát", "Chạy bước 4: xuất file") | next action from `stage` | §4.1 (+ §4.2, §4.3, §4.6 on click) | on mount; every 1 s while `CUTOVER_DONE`; after any action |
| "Tiến trình chốt ngày": 4 steps, badges "Xong" / "Tiếp theo" / "Chờ", inline refusal alert | `stage`; business-date labels; a failed action's problem `detail` | §4.1 | as above |
| "So nhanh tổng số liệu hai bên" (Easy) / "So sánh 0500 và số liệu issuer" (Expert), "Khớp {matched} / {total} giao dịch", match meter | `totals[]`, `matchedCount`, `totalCount` | §4.1 | as above |
| "Các chênh lệch cần xử lý", "{count} chưa xử lý" / "Đã xử lý hết" | `ReconBreak[]`, `openBreaks` | §4.4 (enabled from `RECONCILED`), §4.5 on click | after each resolve (invalidates `["settlement"]`) |
| Net position card "Ngân hàng thanh toán sẽ nhận" (Easy) / "net_position · participant 970499 · {dd/MM}" (Expert) | `netPosition` | §4.1 | as above |
| "File quyết toán" | `clearingFile` | §4.1 (and the §4.6 response) | as above |
| "Chưa có dịch vụ đối soát trên hệ thống này" | the upstream error text | §4.1 failure | once (no retry on 4xx) |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency (If-Match/ETag) | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/settlement/days/{businessDate}` | settlement (planned) | Day stage, totals, counts, net position, file | on mount; 1000 ms polling while `stage == CUTOVER_DONE`; invalidated after every action | n/a | none | Planned MCN-704 (MCN-705 is its consumer); dev:mock only |
| 4.2 | `POST /v1/settlement/days/{businessDate}/cutover` | settlement → gateway (planned) | Step 1: cutover (0800/201) | "Chạy bước 1: khóa sổ" | required, new UUID per click | stage precondition (409) | Planned MCN-702; dev:mock only |
| 4.3 | `POST /v1/settlement/days/{businessDate}/reconciliations` | settlement (planned) | Step 3: run the reconciliation | "Chạy bước 3: đối soát" | required | stage precondition (409) | Planned MCN-704; dev:mock only |
| 4.4 | `GET /v1/settlement/days/{businessDate}/breaks` | settlement (planned) | The break list | enabled once `stage` ≥ `RECONCILED`; refetched after every action | n/a | none | Planned MCN-704; dev:mock only |
| 4.5 | `POST /v1/settlement/breaks/{breakId}/resolutions` | settlement (planned) | Resolve one break (audited) | the break's action button | required | none | Planned MCN-704; dev:mock only |
| 4.6 | `POST /v1/settlement/days/{businessDate}/clearing-files` | settlement (planned) | Step 4: generate the clearing file; 409 `open-breaks` while breaks are open | "Chạy bước 4: xuất file" | required | stage and open-breaks preconditions (409) | Planned MCN-704; dev:mock only |
| – | Step 2 (0500/0510 totals) | gateway, internal | Follows the cutover asynchronously; no endpoint (Ruling R4) | – | – | – | Planned MCN-702 |
| – | WebSocket | – | none | – | – | – | – |

**Stage machine** (`SettlementStage`; the step index equals the stage index, Ruling R3):

| `stage` | Steps shown (1–4) | Primary button | Reached by |
| --- | --- | --- | --- |
| `OPEN` | current, pending, pending, pending | "Chạy bước 1: khóa sổ" → §4.2 | day start |
| `CUTOVER_DONE` | done, current, pending, pending | "Đang gửi tổng kết…" (disabled); §4.1 polled every 1 s | §4.2 |
| `TOTALS_EXCHANGED` | done, done, current, pending | "Chạy bước 3: đối soát" → §4.3 | the gateway's 0500/0510 after the cutover (MCN-702-AC2) |
| `RECONCILED` | done, done, done, current | "Chạy bước 4: xuất file" → §4.6 | §4.3 |
| `FILE_GENERATED` | all done | none (a closed day can't be reopened, Ruling R5) | §4.6 |

## 4. Calls

Common path parameter:

| name | in | type | required | constraints | default |
| --- | --- | --- | --- | --- | --- |
| `businessDate` | path | string, `format: date` (`YYYY-MM-DD`) | ✓ | – | the UI sends the **viewer's local calendar day** (`localBusinessDate()`, Ruling R11) |

Common headers on the POSTs: `Idempotency-Key` (UUID, required, a new `crypto.randomUUID()` per click), `traceparent` (optional, forwarded by the BFF). The POSTs in §4.2, §4.3 and §4.6 have no body.

### 4.1 GET /v1/settlement/days/{businessDate}

**Summary.** The state of one business day's settlement.

**Request.** `businessDate` only.

**Response.** `200`, `SettlementDay`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `businessDate` | date | ✓ | not shown (labels derive from the requested date) | – | – |
| `stage` | `SettlementStage` | ✓ | step badges, primary button, polling (see the stage machine) | step titles "Khóa sổ ngày {dd/MM}", "Gửi bảng tổng kết", "Đối soát từng giao dịch", "Xuất file quyết toán" | step lines "0800 · field 70 = 201 · field 15 = {MMDD+1}", "0500 · fields 74–89, 97 → 0510 RC 95 (out of balance)", "recon_run · khóa ghép (acquirer, TID, STAN, field 7)", "clearing_file · trailer tổng · SHA-256" |
| `totals[]` | `TotalsRow[]` | ✓ | totals table; empty → "Bảng tổng kết sẽ có sau bước 2, khi ngân hàng thanh toán gửi số liệu của mình sang." | see below | see below |
| `totals[].metric` | enum `DEBITS_COUNT`, `DEBITS_AMOUNT`, `DEBIT_REVERSALS_COUNT`, `DEBIT_REVERSALS_AMOUNT`, `NET_AMOUNT` | ✓ | row label | "Số giao dịch ghi nợ", "Tổng tiền ghi nợ", "Số giao dịch đã hủy", "Tổng tiền đã hủy", "Tiền quyết toán ròng" | "Debits, number · F76", "Debits, amount · F88", … |
| `totals[].isoField` | string | ✓ | Expert suffix `F{isoField}` | not shown | shown |
| `totals[].acquirer` | int64 | ✓ | column "Ngân hàng thanh toán" / "Acquirer (0500)" | count, or amount in minor units formatted as ₫ | same |
| `totals[].issuer` | int64 | ✓ | column "Ngân hàng phát hành" / "Issuer" | same | same |
| `totals[].matches` | boolean | ✓ | badge "Khớp" (ok) / "Lệch" (bad) | same | same |
| `matchedCount`, `totalCount` | integer \| null | – | "Khớp {matched} / {total} giao dịch"; meter `scaleX(matched/total)` | shown from `RECONCILED` when both are non-null and `totalCount` > 0 | same |
| `openBreaks` | integer | ✓ | "{count} chưa xử lý" / "Đã xử lý hết" | from `RECONCILED` | same |
| `netPosition` | `Money` \| null | – | net card amount; `null` → "Chưa có số liệu" | "Ngân hàng thanh toán sẽ nhận" + "Đây là số tiền ngân hàng phát hành phải chuyển cho ngân hàng thanh toán sau khi bù trừ các giao dịch đã hủy." | "net_position · participant 970499 · {dd/MM}" + "Σ debits − Σ reversals theo số liệu đã đối soát. Chuyển tiền thật qua ngân hàng quyết toán T+1." |
| `clearingFile` | `ClearingFile` \| null | – | "File quyết toán" card; null → "File sẽ được tạo ở bước 4, sau khi mọi chênh lệch đã được xử lý." | see §4.6 | see §4.6 |

**Provider rules.**
1. `stage` only moves forward along `OPEN → CUTOVER_DONE → TOTALS_EXCHANGED → RECONCILED → FILE_GENERATED` and is persisted in `settlement_day(business_date, stage)` (docs/05 §4).
2. `CUTOVER_DONE → TOTALS_EXCHANGED` happens without a client call, when the 0510 for the closed date has been stored in `recon_totals` (MCN-702-AC2). The client polls every 1000 ms only while `stage == CUTOVER_DONE`. dev:mock makes the transition 1 200 ms after the cutover (`TOTALS_EXCHANGE_DELAY_MS`).
3. `totals` is `[]` before `TOTALS_EXCHANGED`. From then on it has one row per metric, in the order above. The ISO fields are 76, 88, 77, 89 and 97 (docs/03 §7.7).
4. `acquirer` and `issuer` are counts for `*_COUNT` metrics, and minor units in the day's currency for the others. `TotalsRow` carries no currency: the UI uses `netPosition.currency`, or `704` when that's null (Ruling R7).
5. `matches` is computed by the provider; the UI never compares the numbers itself (Ruling R6). Once every break is resolved, all rows must report `matches: true` (MCN-704-AC2). dev:mock then sets `issuer = acquirer`.
6. `matchedCount`, `totalCount` and `openBreaks` count reconciled transactions and still-`OPEN` breaks. Before `RECONCILED`, `openBreaks` is 0 and both counts are null.
7. `netPosition` = Σ debits − Σ reversals over the reconciled data (MCN-704-AC3). It's positive when the issuer pays the acquirer.
8. `clearingFile` is non-null from `FILE_GENERATED`.

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 404 | none today (plain text `404 page not found` from the gateway) | no provider (real stack) | "Chưa có dịch vụ đối soát trên hệ thống này" card + "Phản hồi từ hệ thống: 404 page not found"; no retry on a 4xx |
| 404 | `not-found` (planned) | unknown business date | same card |
| 5xx / 502 `upstream-unavailable` | – | provider or BFF failure | up to 3 retries, then the same card with the detail |

**Example** (dev:mock at the initial stage `TOTALS_EXCHANGED`, trimmed):
```http
GET /api/v1/settlement/days/2026-09-25
200 OK
{"businessDate":"2026-09-25","stage":"TOTALS_EXCHANGED",
 "totals":[{"metric":"DEBITS_COUNT","isoField":"76","acquirer":1282,"issuer":1281,"matches":false},
           {"metric":"DEBITS_AMOUNT","isoField":"88","acquirer":237184500,"issuer":237149500,"matches":false}, …,
           {"metric":"NET_AMOUNT","isoField":"97","acquirer":228164500,"issuer":228729500,"matches":false}],
 "matchedCount":null,"totalCount":null,"openBreaks":0,
 "netPosition":{"amount":228164500,"currency":"704"},"clearingFile":null}
```
Real stack (2026-09-25):
```http
GET /api/v1/settlement/days/2026-09-25
404 Not Found
Content-Type: text/plain; charset=utf-8

404 page not found
```

**Notes.** Query key `["settlement", "day", businessDate]`. `retry`: up to 3, and never for a 4xx `SettlementApiError` (Ruling R12). `refetchInterval` is 1000 ms while `CUTOVER_DONE`, otherwise off. `SettlementApiError.message` is the problem's `detail`, else its `title`, else the raw text body, else `HTTP {status}`.

### 4.2 POST /v1/settlement/days/{businessDate}/cutover

**Summary.** Closes the business day: the gateway sends 0800 with DE 70 = 201 and the next DE 15, then 0500 for the closed date (docs/03 §7.6, MCN-702-AC1).

**Request.** `businessDate`, `Idempotency-Key`; no body.

**Response.** `202`, `SettlementDay` at `CUTOVER_DONE`.

**Provider rules.**
1. Precondition `stage == OPEN`, otherwise `409 conflict`.
2. Messages already in flight keep the DE 15 they were sent with (docs/03 §7.6).
3. The 0500 exchange runs after the response; the day moves to `TOTALS_EXCHANGED` on its own (§4.1 rule 2).
4. Idempotent: replaying the same key returns the stored `202` and body (docs/04 §2).

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 409 | `conflict` | day isn't `OPEN` | the problem `detail` in the steps card's alert, with a shake |
| 400 | `insufficient-idempotency-key` | header missing | same (the UI always sends one) |

**Example** (dev:mock, from `OPEN`):
```http
POST /api/v1/settlement/days/2026-09-25/cutover
Idempotency-Key: 00000000-0000-4000-8000-000000000004

202 Accepted
{"businessDate":"2026-09-25","stage":"CUTOVER_DONE","totals":[],"matchedCount":null,"totalCount":null,"openBreaks":0,"netPosition":{"amount":228164500,"currency":"704"},"clearingFile":null}
```
Refused (dev:mock, day not `OPEN`):
```json
{"type":"https://mcn.local/problems/conflict","title":"Stage transition not allowed","status":409,"detail":"The day is TOTALS_EXCHANGED, not OPEN."}
```

**Notes.** On success every `["settlement"]` query is invalidated. Mutations don't retry. The button is disabled while the request is pending.

### 4.3 POST /v1/settlement/days/{businessDate}/reconciliations

**Summary.** Runs the transaction-level reconciliation for the day (`reconcile()`, MCN-704-AC1), producing breaks.

**Request.** `businessDate`, `Idempotency-Key`; no body.

**Response.** `202`, `SettlementDay` at `RECONCILED`.

**Provider rules.**
1. Precondition `stage == TOTALS_EXCHANGED`, otherwise `409 conflict`.
2. Transactions match on (acquirer, TID, STAN, field 7), as the Expert step text says: "recon_run · khóa ghép (acquirer, TID, STAN, field 7)". Every `BreakType` must be producible.
3. After it, `matchedCount`, `totalCount` and `openBreaks` are set, and §4.4 returns the breaks.

**Errors.** As §4.2, with 409 when the day isn't `TOTALS_EXCHANGED`.

**Example** (dev:mock): `POST …/reconciliations` → `202` with `"stage":"RECONCILED","matchedCount":1279,"totalCount":1282,"openBreaks":3`.

**Notes.** As §4.2.

### 4.4 GET /v1/settlement/days/{businessDate}/breaks

**Summary.** The day's reconciliation breaks.

**Request.** `businessDate` only. There's no paging and no filter.

**Response.** `200`, `ReconBreak[]`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `breakId` | string | ✓ | row key; §4.5 path | – | – |
| `breakType` | enum `MISSING_AT_ISSUER`, `MISSING_AT_ACQUIRER`, `AMOUNT_MISMATCH`, `STATUS_MISMATCH`, `DUPLICATE` | ✓ | badge; action and done labels; resolution mapping | "Thiếu ở ngân hàng phát hành", "Thiếu ở ngân hàng thanh toán", "Lệch số tiền", "Lệch trạng thái", "Giao dịch bị trùng" | the enum value |
| `rrn` | string | ✓ | reference | "Mã tra soát {rrn}" | "RRN {rrn}" |
| `amountDiff` | `Money` | ✓ | amount at the right | formatted ₫ | same |
| `easyText` | string | – | description | shown (falls back to `technicalText`) | fallback |
| `technicalText` | string | – | description | fallback | shown (falls back to `easyText`) |
| `resolution` | `OPEN` \| `AUTO_RESOLVED` \| `MANUAL_ADJUSTED` \| `WRITTEN_OFF` | ✓ | `OPEN` → action button; otherwise the done badge | action "Yêu cầu ngân hàng phát hành kiểm tra", "Điều chỉnh theo số tiền chốt", "Khớp lại khi lệnh hủy tới nơi", "Loại bỏ bản trùng", …; done "Đã gửi yêu cầu kiểm tra", "Đã điều chỉnh", "Đã tự khớp lại", "Đã loại bỏ bản trùng" | same |
| `resolvedBy` | string \| null | – | not shown | – | – |

**Provider rules.**
1. Returns `[]` before `RECONCILED`.
2. Resolved breaks stay in the list with their `resolution`; the UI keeps them, fading them to a done state.
3. The order is the provider's; the UI keeps it. dev:mock returns the canvas's three breaks: `brk-626514000131` MISSING_AT_ISSUER 185 000, `brk-626514000124` STATUS_MISMATCH 600 000, and `brk-626514000098` AMOUNT_MISMATCH 150 000.
4. `easyText` and `technicalText` are provider copy in one language (Vietnamese in dev:mock). The type, action and done labels come from `messages/*.json` (Ruling R8).
5. RRNs are 12 characters. There's no PAN or other card data.

**Errors.** Same availability behaviour as §4.1. A failure here leaves the panel empty (breaks default to `[]`).

**Example** (dev:mock, from `RECONCILED`, one item):
```json
[{"breakId":"brk-626514000124","breakType":"STATUS_MISMATCH","rrn":"626514000124","amountDiff":{"amount":600000,"currency":"704"},
  "easyText":"Một bên ghi đã hủy, bên kia vẫn ghi đã duyệt vì lệnh hủy còn nằm trong hàng đợi lúc khóa sổ.",
  "technicalText":"acquirer REVERSED · issuer APPROVED · 0420 trong SAF tại thời điểm cutover","resolution":"OPEN","resolvedBy":null}, …]
```

**Notes.** Query key `["settlement", "breaks", businessDate]`, enabled only from `RECONCILED`. TanStack default retries.

### 4.5 POST /v1/settlement/breaks/{breakId}/resolutions

**Summary.** Resolves one break with a resolution and a reason (audited, MCN-704-AC2).

**Request.**

| name | in | type | required | constraints | notes |
| --- | --- | --- | --- | --- | --- |
| `breakId` | path | string | ✓ | – | |
| `Idempotency-Key` | header | UUID | ✓ | – | new per click |

Body:

| field | type | required | constraints | notes |
| --- | --- | --- | --- | --- |
| `resolution` | enum `AUTO_RESOLVED`, `MANUAL_ADJUSTED`, `WRITTEN_OFF` | ✓ | – | the UI maps by type: `STATUS_MISMATCH` → `AUTO_RESOLVED`, `DUPLICATE` → `WRITTEN_OFF`, every other type → `MANUAL_ADJUSTED` (Ruling R8, `resolutionFor`) |
| `note` | string | – | maxLength 500 | the UI sends the action's own label, for example "Điều chỉnh theo số tiền chốt" (the canvas has no reason input) |

**Response.** `200`, the updated `ReconBreak`.

**Provider rules.**
1. It sets `resolution` and `resolvedBy` (the actor) and writes an audit record with the reason (MCN-704-AC2).
2. The day's `openBreaks`, `matchedCount` and `totals[].matches` reflect the change on the next §4.1 read.
3. Unknown `breakId` → `404 not-found`.

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 404 | `not-found` | unknown break | the `detail` in the steps card's alert, with a shake; the row's button is enabled again |
| 400 | `validation-error` (planned) | bad `resolution` or `note` too long | same |

**Example** (dev:mock):
```http
POST /api/v1/settlement/breaks/brk-626514000098/resolutions
Idempotency-Key: 00000000-0000-4000-8000-000000000005
Content-Type: application/json

{"resolution":"MANUAL_ADJUSTED","note":"Điều chỉnh theo số tiền chốt"}

200 OK
{"breakId":"brk-626514000098","breakType":"AMOUNT_MISMATCH","rrn":"626514000098","amountDiff":{"amount":150000,"currency":"704"}, …,"resolution":"MANUAL_ADJUSTED","resolvedBy":"ops.demo"}
```

**Notes.** One resolve runs at a time per row (`resolvingId`). On success every `["settlement"]` query is invalidated, which refetches the day and the breaks.

### 4.6 POST /v1/settlement/days/{businessDate}/clearing-files

**Summary.** Generates the day's clearing file, refused while any break is open.

**Request.** `businessDate`, `Idempotency-Key`; no body.

**Response.** `201`, `ClearingFile`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `fileName` | string | ✓ | file name (JetBrains Mono 500) | shown | shown |
| `recordCount` | integer | ✓ | "{records} dòng · {total}"; row "Số dòng" = "{records} + trailer" | shown | shown |
| `total` | `Money` | ✓ | summary in ₫; row "Tổng tiền" in plain minor units (the trailer's format) | shown | shown |
| `sha256` | string `^[0-9a-f]{64}$` | ✓ | "Mã kiểm tra file" / "SHA-256": first 6 + `…` + last 4; full value in `title` | shown | shown |
| `status` | `GENERATED` \| `SENT` \| `ACKNOWLEDGED` \| `REJECTED` | ✓ | not shown | – | – |

**Provider rules.**
1. Precondition `stage == RECONCILED`, otherwise `409 conflict`.
2. **With any `OPEN` break the request is refused with `409 open-breaks`** (docs/04 §3; MCN-704-AC3). The UI shows the problem's `detail` verbatim in the steps card, shaken twice, and shakes it again on each new attempt (MCN-705-AC2, Ruling R9). The `detail` must therefore be written for an operator.
3. On success: a deterministic CSV plus a trailer carrying the totals, its SHA-256, and the net position (MCN-704-AC3). The stage becomes `FILE_GENERATED`, and `SettlementDay.clearingFile` holds the same object.
4. dev:mock names the file `CLR_970499_<YYYYMMDD>_001.csv`. The contract doesn't fix a name pattern.

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 409 | `open-breaks` | at least one `OPEN` break | the `detail` inline in the steps card with a shake (MCN-705-AC2) |
| 409 | `conflict` | day isn't `RECONCILED` | same |

**Example** (dev:mock). Refused with 2 breaks open:
```json
{"type":"https://mcn.local/problems/open-breaks","title":"Open breaks remain","status":409,
 "detail":"Còn 2 chênh lệch chưa xử lý. Hãy xử lý hết trước khi xuất file quyết toán."}
```
Accepted after all three are resolved:
```json
{"fileName":"CLR_970499_20260925_001.csv","recordCount":1282,"total":{"amount":228164500,"currency":"704"},
 "sha256":"a91f034b8d4b8d…4b8dd27c2e","status":"GENERATED"}
```
(The dev:mock `sha256` is a fixed placeholder, not a real digest.)

**Notes.** As §4.2.

## 5. Real-time events

None. `contracts/ws-events.schema.json` defines no settlement event. Stage changes that happen without a user action (the 0500 exchange) are observed by polling §4.1 every 1000 ms while `CUTOVER_DONE`.

## 6. Security and compliance

| Topic | Rule on this page |
| --- | --- |
| Card data | No PAN, masked PAN, track data, CVV or PIN data on any settlement call. Breaks are identified by RRN. The clearing file's contents never reach the browser, only its metadata and hash. |
| Money | Integer minor units only. Counts and amounts in `TotalsRow` are `int64`. |
| Audit | Break resolutions are audited with actor and reason (MCN-704-AC2). The actor should come from the BFF's `X-Actor` (docs/04 §2), which the BFF doesn't send yet (see the Cards page, CARDS-G3). |
| Destructive or irreversible actions | Cutover and clearing-file generation can't be undone. No reopen exists (Ruling R5). They're guarded by stage preconditions (409) and, for the file, by `open-breaks`. The UI has no extra confirmation step, as in the canvas. |
| Idempotency | Every POST carries a fresh `Idempotency-Key` per click. A replay must return the stored response. |

## 7. Non-functional requirements

No NFR sets a latency for settlement calls, and no provider exists to observe. Targets for the provider:

| Call | Target | Basis |
| --- | --- | --- |
| 4.1 | fast enough for 1 s polling; one indexed read of `settlement_day` + `recon_totals` | Ruling R4 cadence |
| 4.2 | returns `202` without waiting for the 0500/0510 | MCN-702-AC2, Ruling R4 |
| 4.3 | returns `202`; may run the reconciliation inline for a lab-sized day | contract `202` |
| 4.6 | returns `201` with the file metadata | contract |

- Polling load: 1 request per second per open page, only while `CUTOVER_DONE`.
- Payload bounds: `SettlementDay` ≈ 1 KB (5 totals rows). `ReconBreak[]` is unbounded and unpaged (§9 SET-G7).
- Pagination: none.

## 8. UI states

| State | Driver | What renders |
| --- | --- | --- |
| Loading | 4.1 pending | heading + "Đang tải số liệu chốt ngày…" |
| Provider not available | 4.1 fails (real stack: 404 at once; 5xx after 3 retries) | heading + the card "Chưa có dịch vụ đối soát trên hệ thống này", body "Chốt ngày và đối soát cần dịch vụ settlement, sẽ có từ bản R7 cùng MCN‑702 và MCN‑704. Chạy pnpm dev:mock để xem màn hình này với số liệu mẫu.", then "Phản hồi từ hệ thống: {detail}" (Ruling R12) |
| Empty totals | `totals == []` (before `TOTALS_EXCHANGED`) | "Bảng tổng kết sẽ có sau bước 2, …" |
| Empty breaks | before `RECONCILED` | "Danh sách chênh lệch sẽ có sau bước 3, …" |
| Empty file | `clearingFile == null` | "File sẽ được tạo ở bước 4, …" |
| Waiting for totals | `CUTOVER_DONE` | disabled "Đang gửi tổng kết…"; polling |
| Action refused | 409 / 4xx from 4.2, 4.3, 4.5 or 4.6 | the problem `detail` in an alert under the steps, with a shake |
| Partial | 4.4 fails while 4.1 succeeds | the breaks panel shows its header and meta with an empty list |
| Done | `FILE_GENERATED` | all four steps done; no primary button; file card filled |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| SET-G1 | No provider serves `/v1/settlement*`. The gateway answers `404 page not found` (text/plain). The settlement service is up on :8082 but has no such routes (Spring JSON 404). | live GETs on 2026-09-25: `:3000/api/v1/settlement/days/2026-09-25` → 404, `:8080/…` → 404, `:8082/…` → 404 | SET + GW | MCN-702, MCN-703, MCN-704 (Sprint 9) |
| SET-G2 | The BFF routes `/v1/settlement` to the gateway, while docs/04 §1 assigns it to `settlement:8082` | `route.ts:14-16` | WEB | Add `SETTLEMENT_URL` and route `/v1/settlement*` to it when MCN-704 lands |
| SET-G3 | The contract declares no 409 responses: `open-breaks` and `conflict` exist only in docs/04 §3 and dev:mock. The stage preconditions for each POST aren't written in `openapi.yaml`. | `openapi.yaml` settlement paths (only `default: Problem`) | PLAT (contracts) | Add explicit `409` responses with examples, and state the preconditions in the descriptions |
| SET-G4 | Step 2 has no endpoint; progress depends on 1 s polling and on MCN-702 advancing the stage by itself | Ruling R4 | GW | MCN-702-AC2 must update `settlement_day.stage` (or settlement must derive it from `recon_totals`) |
| SET-G5 | dev:mock simplifications a real provider must not copy: it starts at `TOTALS_EXCHANGED`; returns `netPosition` at every stage; ignores `Idempotency-Key`; resolves breaks without a stage check; fixes `resolvedBy` to `"ops.demo"`; uses a placeholder `sha256` | `mocks/pages/settlement.ts:62-86, 98-141` | WEB | Keep as mock-only behaviour; contract tests against the real provider (MCN-704) |
| SET-G6 | The UI's business date is the viewer's local calendar day, not the network's business date. Around cutover, or in another time zone, the page can address the wrong day. | `settlement-model.ts` `localBusinessDate`; Ruling R11 | SET + WEB | Expose the current business date (for example `GET /v1/settlement/days/current`, or a field on an existing resource) and use it |
| SET-G7 | No list-days endpoint and no paging on breaks | `openapi.yaml` | PLAT (contracts) | Add them when multi-day history is in scope |
| SET-G8 | No `FF_S7_SETTLEMENT` check: the page ships unflagged | Ruling R13 | WEB | Read slice flags when web-next gains flag support |
| SET-G9 | `TotalsRow` has no currency, and `SettlementDay` has no participant id (the UI hardcodes `participant 970499`) | `openapi.yaml` `TotalsRow`, `SettlementDay`; Rulings R7, R11 | PLAT (contracts) | Add `currency` to `SettlementDay` and `participantId`, additively |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version (Draft): the contract and dev:mock behaviour documented ahead of the provider (MCN-702/703/704). |
