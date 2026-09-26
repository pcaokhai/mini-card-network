# Security and keys page: API contract

| | |
| --- | --- |
| Document | `docs/api/security-page.md` |
| Version | 1.5 |
| Status | Approved for integration |
| Date | 2026-09-25 |
| Screen | route `/security`, container `web-next/src/app/(console)/security/SecurityScreen.tsx` |
| Stories | MCN-505 (WEB); MCN-504 (GW + ISS, dynamic key exchange); MCN-502 / MCN-503 (PIN and MAC, which the rotated keys serve) |
| Provider(s) | gateway-go (`gateway-go/internal/api/keys.go`, `gateway-go/internal/api/rotations.go`, `gateway-go/internal/rotation/`, wired in `gateway-go/cmd/gateway/main.go`); the issuer-jpos side of the key change is ISO 8583 only |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/security-client.ts` |
| Design reference | canvas `project/Security.dc.html` (docs/01-prd.md §7.1); plan `docs/plans/MCN-505-security-canvas.md` |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only). No rotation was started, so rotation examples come from dev:mock (`web-next/src/mocks/pages/security.ts`) and are labelled. |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and are not repeated.

## 1. Purpose and scope

The page shows the acquirer's working keys by check value (KCV) only, with their lifetime and status. It lets the operator rotate the ZPK and watch the four rotation steps, and teaches how a PIN block is built. This contract covers the three gateway calls the page makes.

Out of scope:
- `GET /v1/keys/issuer`: the page doesn't call it and the BFF doesn't route it to the issuer (§9 SEC-G1).
- The PIN-block visualiser and the PCI "never do" list: both are client-only (§3).
- ZAK rotation: the contract allows it, but the screen offers only ZPK (Ruling 3).

## 2. Page map

| UI region (canvas / i18n label) | Data shown | Call(s) | Refresh |
| --- | --- | --- | --- |
| Title "Bảo mật và khóa" + subtitle | static | none | none |
| "Khóa đang sử dụng" table: "Khóa", "Mã kiểm tra", "Thời hạn còn lại", "Trạng thái", "Xoay khóa ngay" | one row per key: name, KCV, lifetime bar, status badge | §4.1 | on mount; after a rotation reports `COMPLETED` |
| "Xoay khóa mã hóa PIN" stepper, button "Bắt đầu xoay khóa ZPK" / "Đang xoay…" / "Hoàn tất · làm lại" | four steps and their status; the new KCV in Expert | §4.2, §4.3 | 1 s polling while `RUNNING` |
| "Mã PIN được bảo vệ thế nào" | ISO 9564 format 0 rows for a typed PIN on an illustrative card | client only; the ACTIVE ZPK KCV from §4.1 salts the last row | on "Tạo PIN block" |
| "Những điều hệ thống không bao giờ làm" | six static PCI statements | none | none |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency (If-Match/ETag) | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/keys/acquirer` | gateway-go | Key inventory: type, KCV, status, lifetime | on mount; refetch on window focus; invalidated when §4.3 returns `COMPLETED` | n/a | none | Real |
| 4.2 | `POST /v1/keys/acquirer/rotations` | gateway-go | Start a ZPK rotation | "Xoay khóa ngay" or "Bắt đầu xoay khóa ZPK" | required (UUID per click); presence-checked only (§9 SEC-G4) | none | Real (verified in dev:mock only) |
| 4.3 | `GET /v1/keys/acquirer/rotations/{rotationId}` | gateway-go | Rotation status and steps | once after §4.2; then every 1000 ms while `status == RUNNING` | n/a | none | Real |
| – | `GET /v1/keys/issuer` | issuer-jpos (:8081) | Issuer key inventory | not called | – | – | Served on :8081 only; via the BFF it reaches the gateway, which answers 404 |
| – | PIN-block visualiser | client only | Format 0 PIN field, PAN field, clear block, two illustrative cipher rows | on submit | – | – | Client only |
| – | PCI "never do" list | client only | Static copy | – | – | – | Client only |
| – | WebSocket | – | none | – | – | – | – |

The BFF sends every `/api/v1/keys/*` path to `GATEWAY_URL`: `upstreamFor()` routes only `/v1/cards` to the issuer (`route.ts:14-16`).

## 4. Calls

### 4.1 GET /v1/keys/acquirer

**Summary.** Every row of the gateway's `key_store`, as `KeyInfo`, without key material.

**Request.** No parameters or body.

**Response.** `200`, array of `KeyInfo`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `keyType` | enum `ZMK`, `ZPK`, `ZAK`, `TPK`, `TAK`, `CVK`, `PVK` | ✓ | row name and note; row order (ZMK, ZPK, ZAK, TPK, TAK, CVK, PVK) | name, for example "Khóa mã hóa PIN giữa hai ngân hàng", with the note "Bảo vệ PIN trên đường truyền liên ngân hàng" | `ZPK · {Easy name}`, with the note "under LMK · {lifetimeDays} ngày/chu kỳ" |
| `counterparty` | string \| null | ✓ (nullable) | not shown | – | – |
| `kcv` | string `^[0-9A-F]{6}$` | ✓ | "Mã kiểm tra" cell; it flips when it equals the completed rotation's `newKcv` | as sent | as sent |
| `status` | `PENDING` \| `ACTIVE` \| `RETIRED` | ✓ | badge (with the lifetime, rule 4) | "Đang dùng" / "Sắp đến hạn" / "Đang chờ" / "Đã thu hồi" | `ACTIVE` / `ROTATE SOON` / `PENDING` / `RETIRED` |
| `activatedAt` | RFC 3339 \| null | – | not shown | – | – |
| `daysRemaining` | integer | ✓ | "Còn {days} ngày" and the bar `scaleX(daysRemaining / lifetimeDays)` | same | same |
| `lifetimeDays` | integer | ✓ | bar denominator; the Expert note | – | "{days} ngày/chu kỳ" |

**Provider rules.**
1. **KCV only.** The response is built from `store.KeyRow` through `keyInfo`, which has no field for `key_under_lmk` (`keys.go:23-32`). Clear keys and wrapped keys never leave the gateway.
2. **Rows.** One entry per `key_store` row, in `id` order, with every status: the initial ACTIVE ZAK and ZPK, plus any `RETIRED` row and any `PENDING` row a failed rotation left behind (§9 SEC-G8). The acquirer `key_store` accepts only `ZMK`, `ZPK`, `ZAK`, `TPK` and `TAK` (migration `00004_key_store.sql`). On the local stack it holds ZAK and ZPK only.
3. **Lifetime.** `lifetimeDays` = 365 for every key type (`keyLifetimeDays`). `daysRemaining` = 365 − whole days since `activatedAt`, floored at 0, or 365 when `activatedAt` is null.
4. **Status as shown (client derivation).** `PENDING` → pending; `RETIRED` → retired; `ACTIVE` with `daysRemaining / lifetimeDays` < 20 % → "Sắp đến hạn" / `ROTATE SOON` (warn tone); other `ACTIVE` → "Đang dùng" (`security-model.ts` `statusOf`).
5. `counterparty` is `owner_ref`, which is null for the gateway's single global key per type.
6. The screen picks the row with `keyType == ZPK` and `status == ACTIVE` as the KCV for the visualiser's last row.

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 500 | `list-keys-failed` | `key_store` query failed (the detail is the database error text) | alert "Không tải được danh sách khóa. Thử tải lại trang."; the table renders headers only |
| 502 | `upstream-unavailable` (BFF) | gateway not reachable | same |

**Example** (real, local stack 2026-09-25):
```http
GET /api/v1/keys/acquirer
200 OK
[{"keyType":"ZAK","counterparty":null,"kcv":"D927EE","status":"ACTIVE","activatedAt":"2026-09-25T06:29:06.530923Z","daysRemaining":365,"lifetimeDays":365},
 {"keyType":"ZPK","counterparty":null,"kcv":"3B84E8","status":"ACTIVE","activatedAt":"2026-09-25T06:29:06.53306Z","daysRemaining":365,"lifetimeDays":365}]
```
dev:mock serves the canvas's six keys instead (ZMK `8C21D4` 312/365, ZPK `3F9A21` 26/30, ZAK `51E0A7` 5/30, TPK `A4F3C9` 74/90, CVK `0B77E2` 200/365, PVK `D19E40` 200/365), all with `counterparty` `"issuer"` or null.

**Notes.** Query key `["security", "keys", "acquirer"]`. TanStack defaults (`staleTime` 0, 3 retries, refetch on focus). No cache headers. Rate limit: not enforced.

### 4.2 POST /v1/keys/acquirer/rotations

**Summary.** Runs a key rotation for the acquirer's working key: GENERATE → SEND_0800_161 → PARTNER_CONFIRM → ACTIVATE.

**Request.**

| name | in | type | required | constraints | notes |
| --- | --- | --- | --- | --- | --- |
| `Idempotency-Key` | header | UUID | ✓ | the provider checks non-empty only | new `crypto.randomUUID()` per click |
| `Content-Type` | header | string | ✓ | `application/json` | |
| `traceparent` | header | string | – | forwarded by the BFF | |

Body:

| field | type | required | constraints | notes |
| --- | --- | --- | --- | --- |
| `keyType` | enum `ZPK`, `ZAK` | ✓ | the contract enum; the provider relies on the `key_rotation.key_type` CHECK constraint (§9 SEC-G5) | the UI always sends `ZPK` (Ruling 3) |

**Response.** `202`, `KeyRotation`.

| field | type | required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `rotationId` | string (decimal `key_rotation.id`) | ✓ | stored; drives §4.3 | – | – |
| `keyType` | string | ✓ | not shown | – | – |
| `status` | `RUNNING` \| `COMPLETED` \| `FAILED` | ✓ | button label and phase | "Đang xoay…" while running; "Hoàn tất · làm lại" when completed | same |
| `newKcv` | string \| null | ✓ (nullable) | Expert step text; KCV flip in the key table | – | "GenerateKey ZPK under LMK · KCV {kcv}", "0810 RC 00 · KCV match {kcv}" (`······` while null) |
| `steps[].name` | enum | ✓ | the four steps, always drawn in the fixed order | "HSM tạo khóa mới", "Gửi khóa mới cho đối tác", "Đối tác xác nhận", "Kích hoạt khóa mới" | the same titles; details "0800 · field 70 = 161 · ZPK under ZMK", "PENDING → ACTIVE · khóa cũ → RETIRED (grace 5 phút)" |
| `steps[].status` | `PENDING` \| `DONE` \| `FAILED` | ✓ | step dot `data-status`; screen-reader suffix "Đang chờ" / "Xong" / "Thất bại"; a FAILED step reads "Xoay khóa dừng ở bước này. Khóa cũ vẫn đang dùng." | same | same |
| `steps[].completedAt` | RFC 3339 \| null | – | not shown | – | – |

**Provider rules (the step machine, `rotation/runner.go`).**
1. **Create.** It inserts `key_rotation(key_type, status='RUNNING', steps=[4 × PENDING])`.
2. **GENERATE.** It makes 16 random bytes, wraps them under the LMK (`hsm.WrapUnderLMK`), computes the KCV, and inserts a `key_store` row with status `PENDING`. The clear key lives only in memory for the rest of the run and is never logged or persisted.
3. **SEND_0800_161.** It wraps the clear key under the ZMK (AES-256-GCM, `store.EncryptBytes`) and sends a 0800 on the live issuer link with DE 7, DE 11, DE 70 = `161`, and DE 48 = `"<keyType>:" + lowercase hex(cryptogram)`. There's no DE 53. If the link isn't signed on, the step fails ("issuer link not signed on"). docs/03 §7.1 and §11 describe the key-change message.
4. **PARTNER_CONFIRM.** The step succeeds only if the 0810 has DE 39 = `00`.
5. **ACTIVATE.** In one transaction, the prior `ACTIVE` row of the same type becomes `RETIRED` (`retired_at = now()`) and the new row becomes `ACTIVE` (`KeyStoreRepository.Activate`). The rotation becomes `COMPLETED` with `new_kcv`.
6. **Dual-key window.** For 5 minutes after the old key is retired it's still accepted: by the gateway for inbound MAC on the ZAK (`purchase/service.go` `dualKeyWindow`), and by the issuer for the ZAK (MAC) and the ZPK (PVV check) (`KeyStoreRepository.DUAL_KEY_WINDOW`, `VerifySecurity.java`). This is docs/03 §9 "Reversal grace for new key" and MCN-504-AC2. The ACTIVATE step's Easy copy says so: "Khóa cũ vẫn được chấp nhận thêm 5 phút để giao dịch đang bay không bị lỗi."
7. **Audit.** Each step writes a `key_rotation.step` row to the gateway's `audit_log` (`{rotationId, step, status[, error]}`) (MCN-504-AC3).
8. **Synchronous.** The adapter calls `Runner.Run` inline (`main.go` `rotationAdapter`). The response is sent only after all four steps, so a successful call returns `202` with `status: COMPLETED`, never `RUNNING` (§9 SEC-G2).
9. **Failure.** On a step error the runner marks that step and the rotation `FAILED`, but the handler answers `500 rotation-failed` with the error text instead of the `FAILED` row (§9 SEC-G3).

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 400 | `idempotency-key-required` | header missing | alert "Không bắt đầu xoay khóa được. Thử lại sau."; the button returns to "Bắt đầu xoay khóa ZPK" |
| 400 | `invalid-request` | body isn't JSON | same |
| 500 | `rotation-failed` | any step failed (link not signed on, RC ≠ 00, HSM or DB error), or `keyType` outside ZPK/ZAK (CHECK violation) | same; the steps stay as last drawn, because the FAILED row isn't returned |
| 502 | `upstream-unavailable` (BFF) | gateway not reachable | same |

The gateway's problem `type` is the bare slug, not a `https://mcn.local/problems/…` URI (§9 SEC-G7).

**Example** (dev:mock: the mock advances one step every 1 200 ms, so the first answer is `RUNNING`; the real provider answers only when the rotation has finished):
```http
POST /api/v1/keys/acquirer/rotations
Idempotency-Key: 00000000-0000-4000-8000-000000000003
Content-Type: application/json

{"keyType":"ZPK"}

202 Accepted
{"rotationId":"rot_00000000-0000-4000-8000-000000000003","keyType":"ZPK","status":"RUNNING","newKcv":null,
 "steps":[{"name":"GENERATE","status":"DONE","completedAt":null},{"name":"SEND_0800_161","status":"PENDING","completedAt":null},
          {"name":"PARTNER_CONFIRM","status":"PENDING","completedAt":null},{"name":"ACTIVATE","status":"PENDING","completedAt":null}]}
```
The real `rotationId` is a decimal string (for example `"7"`). The dev:mock id is `rot_<Idempotency-Key>`, so a replay lands on the same rotation.

**Notes.** On success the client seeds `["security", "rotations", rotationId]` with the response and `useZpkRotation` starts §4.3. The mutation has no retry. Only one rotation is tracked per page view. "Hoàn tất · làm lại" resets the view to idle and doesn't rotate again. Rate limit: not enforced; nothing stops concurrent rotations.

### 4.3 GET /v1/keys/acquirer/rotations/{rotationId}

**Summary.** A rotation's current status and steps.

**Request.**

| name | in | type | required | constraints | default |
| --- | --- | --- | --- | --- | --- |
| `rotationId` | path | string | ✓ | the provider parses it as a base-10 int64 | – |

**Response.** `200`, `KeyRotation` (fields and UI mapping as in §4.2).

**Provider rules.**
1. It reads `key_rotation` by id. The step order in `steps[]` is the order stored at create time; the UI draws steps in its own fixed order and matches them by `name`.
2. `newKcv` is non-null only once the rotation is `COMPLETED`.
3. **Client cadence.** `refetchInterval` = 1000 ms while `status == RUNNING`; polling stops on `COMPLETED` or `FAILED`. On `COMPLETED` the client invalidates §4.1 so the table re-reads the key list, and the ZPK KCV cell flips to the new value (MCN-505-AC1).

**Errors.**

| HTTP status | problem `type` slug | when | UI behaviour |
| --- | --- | --- | --- |
| 400 | `invalid-rotation-id` | id isn't an integer | phase "failed"; alert "Không theo dõi được lần xoay khóa này. Tải lại trang để xem khóa đang dùng." |
| 500 | `get-rotation-failed` | unknown id ("no rows in result set") or DB error | same |

**Example** (real, local stack 2026-09-25, error paths; no rotation had run):
```http
GET /api/v1/keys/acquirer/rotations/999999
500
{"detail":"no rows in result set","status":500,"title":"get-rotation-failed","type":"get-rotation-failed"}

GET /api/v1/keys/acquirer/rotations/abc
400
{"detail":"strconv.ParseInt: parsing \"abc\": invalid syntax","status":400,"title":"invalid-rotation-id","type":"invalid-rotation-id"}
```
Completed (dev:mock, after 3 × 1 200 ms):
```json
{"rotationId":"rot_00000000-…","keyType":"ZPK","status":"COMPLETED","newKcv":"7D02B1",
 "steps":[{"name":"GENERATE","status":"DONE","completedAt":null},{"name":"SEND_0800_161","status":"DONE","completedAt":null},
          {"name":"PARTNER_CONFIRM","status":"DONE","completedAt":null},{"name":"ACTIVATE","status":"DONE","completedAt":null}]}
```

**Notes.** Query key `["security", "rotations", rotationId]`, enabled only when an id is held. TanStack default retries (3) apply before the "lost" alert. dev:mock answers an unknown id with a body-less `404`.

## 5. Real-time events

None. The contract defines no key or rotation WebSocket event. Progress comes from polling §4.3.

## 6. Security and compliance

| Topic | Rule on this page |
| --- | --- |
| Key material | **KCV only.** No API on this page returns a clear key, a key under the LMK or a key under the ZMK. The clear key exists only inside `Runner.Run` (docs/04 §2 "Card data", NFR-06). The Expert note says so: "Hiển thị KCV · key material không rời HSM". |
| PIN-block visualiser | Runs entirely in the browser and sends nothing (MCN-505-AC2). It uses the made-up card `9704 36•• •••• 7890` ("Số thẻ minh họa dùng để tính: …"), which isn't in `contracts/fixtures/cards.json` and fails the Luhn check (Ruling 5). The PIN input is `type="password"`, `autoComplete="off"`, and must be 4–12 digits ("Mã PIN gồm 4 đến 12 chữ số, không có chữ cái."). The PIN digits are masked (`•`) in the first row. The last two rows are an FNV-style hash, not 3DES/AES, and the footnote says so: "Hai dòng cuối là giá trị minh họa, không phải kết quả mã hóa 3DES/AES thật." The ZPK KCV only salts that hash. |
| PAN | The page receives no PAN, not even a masked one. |
| Audit | Each rotation step writes an audit row (§4.2 rule 7). The gateway's `audit_log` has no actor column, and the BFF sends no `X-Actor` (§9 SEC-G14). |
| Destructive action | Rotation changes the working key for every user of the stack. The UI gives no confirmation step (the canvas has none). The button is disabled while running. R5.3 forbids rotating on the shared stack. |
| PCI "never do" list | Static copy (MCN-505-AC3). The Expert audit item claims "audit_log append-only · dual control", but no dual control exists (§9 SEC-G14). |

## 7. Non-functional requirements

No NFR in docs/02 §2 sets a latency for key APIs. The figures below are **observed** on the local stack through the BFF, 60 sequential requests, on 2026-09-25.

| Call | p50 | max of 60 (≈ p99) | Payload |
| --- | --- | --- | --- |
| 4.1 | 4.6 ms | 8.5 ms | ≈ 0.3 KB (2 keys); grows by one row per rotation (SEC-G8) |
| 4.2 | not measured (no rotation on the shared stack). Bounded by one 0800/0810 round trip on the MUX plus five DB writes, and by the request context: the call blocks until the run ends. | – | ≈ 0.4 KB |
| 4.3 | error path only: < 10 ms | – | ≈ 0.4 KB |

- Polling: 1 request per second per open page, only while a rotation is `RUNNING`. On the real provider that's at most one poll, because POST already returns a terminal status.
- MCN-504-AC2 targets: transactions during a rotation succeed at 50 TPS (integration test), and the old key is accepted for 5 minutes.
- Pagination: none; the key list is unbounded (SEC-G8).

## 8. UI states

| State | Driver | What renders |
| --- | --- | --- |
| Loading | 4.1 pending | the table with headers and no rows; stepper idle; the visualiser uses an empty KCV salt |
| Empty | 4.1 returns `[]` | headers only; no "Xoay khóa ngay" button (rotation is still possible from the stepper) |
| Error | 4.1 fails | alert "Không tải được danh sách khóa. Thử tải lại trang."; the stepper, visualiser and PCI list still work |
| Start failed | 4.2 non-2xx | alert "Không bắt đầu xoay khóa được. Thử lại sau."; the button is enabled again |
| Running | 4.2 pending, or 4.3 `RUNNING` | "Đang xoay…" (disabled); steps light up as `DONE`; "Xoay khóa ngay" hidden |
| Step failed | 4.3 `FAILED` | the failed step reads "Xoay khóa dừng ở bước này. Khóa cũ vẫn đang dùng."; "Bắt đầu xoay khóa ZPK" enabled |
| Lost track | 4.3 errors after retries | alert "Không theo dõi được lần xoay khóa này. Tải lại trang để xem khóa đang dùng." |
| Completed | 4.3 `COMPLETED` | all steps done; "Hoàn tất · làm lại"; the ZPK KCV flips once §4.1 carries `newKcv` |
| Provider not available | gateway down | 502 `upstream-unavailable` for every call; renders as the error states above |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| SEC-G1 | `GET /v1/keys/issuer` isn't reachable through the BFF: `upstreamFor()` sends it to the gateway, which answers `404 page not found`. docs/04 §1 assigns it to the issuer, which serves it on :8081 (it returns `[]` on the local stack). The screen dropped it (Ruling 1). | `route.ts:14-16`; `curl localhost:3000/api/v1/keys/issuer` → 404; `curl localhost:8081/v1/keys/issuer` → 200 `[]` | WEB | **Fixed** in #124 (routing): the BFF sends `/v1/keys/issuer` and `/v1/accounts/**` to `ISSUER_ADMIN_URL`. The screen stays acquirer-only until the issuer's inventory is meaningful (SEC-G15) |
| SEC-G2 | The rotation is synchronous. POST runs all four steps inline and returns `202` with a terminal status, so the "driven by the rotation resource" polling (MCN-505-AC1) never sees `RUNNING` on the real stack, and the HTTP request blocks for the 0800/0810 round trip. | `main.go` `rotationAdapter.StartRotation` → `Runner.Run`; comment at `main.go:247-249` | GW | **Fixed** in #121: POST answers 202 `RUNNING` with `Location`; the steps run on a goroutine owned by `rotation.Runner.Serve`, cancelled on shutdown; a second concurrent rotation → 409 `conflict` |
| SEC-G3 | A failed rotation returns `500 rotation-failed` with the raw error instead of the `FAILED` `KeyRotation`, so the UI can't show which step failed | `rotations.go:56-60`; `runner.go` `failStep` returns the row, which the handler drops | GW | **Fixed** in #121: a failure lands as the `FAILED` row with the failed step, read through GET |
| SEC-G4 | `Idempotency-Key` is presence-checked only: no replay and no 422 mismatch. A retried POST starts a second rotation. | `rotations.go:45-48` | GW | **Fixed** in #121: UUID key; same key and `keyType` replay the rotation (in memory), a different `keyType` → 422 `idempotency-key-mismatch` |
| SEC-G5 | `keyType` isn't validated at the HTTP layer. Anything but ZPK/ZAK fails the `key_rotation` CHECK and surfaces as `500 rotation-failed`, not `400 validation-error`. | `rotations.go:49-55`; `migrations/00005_key_rotation.sql` | GW | **Fixed** in #121: `keyType` other than ZPK/ZAK → 400 `validation-error` |
| SEC-G6 | An unknown `rotationId` returns `500 get-rotation-failed` ("no rows in result set") instead of `404 not-found` | live `GET …/rotations/999999` → 500 | GW | **Fixed** in #121: unknown or non-numeric `rotationId` → 404 `not-found` |
| SEC-G7 | Gateway problems use bare slugs (`type` = `title` = `"get-rotation-failed"`), not `https://mcn.local/problems/…` URIs. They have no `instance` or `traceId`, and `detail` leaks internal error text (pgx and strconv messages). None of the slugs is in docs/04 §3. | `lab.go:73-77` `problem()`; live responses above | GW | One problem writer with URI types from the docs/04 §3 list; generic `detail` for 500s |
| SEC-G8 | The key list returns every `key_store` row. After the first rotation the ZPK appears twice (ACTIVE + RETIRED), and a failed rotation leaves a `PENDING` row forever. `keyRows()` keys table rows by `keyType`, which gives duplicate React keys, and "Xoay khóa ngay" renders on every ZPK row. | `keystore.go` `List` (no WHERE); `runner.go` `runGenerate` inserts `PENDING` before SEND; `KeyTable.tsx` `key={row.keyType}` | GW + WEB | **Fixed** in #121 (provider): the list returns ACTIVE plus PENDING keys. A PENDING key is either a running rotation's, or one whose rotation ended with an unknown outcome (no 0810 after every attempt), which is kept on purpose (SEC-G16). A rotation the issuer declined, or one that never sent, retires its key. `FindRecentlyRetired` ignores keys never activated. WEB part done in #124: rows keyed by type + KCV, rotation offered only on the active ZPK |
| SEC-G9 | `lifetimeDays` is a fixed 365 for all key types, so the "Sắp đến hạn" warning (MCN-505-AC1) can't fire on the real stack until day 293 | `keys.go:13-16` | GW | **Fixed** in #121: `KEY_LIFETIME_DAYS` policy, default `ZMK=365,ZPK=30,ZAK=30`, validated at startup |
| SEC-G10 | The gateway loads the ZAK once at startup. After a ZAK rotation it keeps computing outbound MACs with the retired ZAK, which the issuer accepts for 5 minutes only, then answers RC 96. The UI doesn't offer ZAK, but the API accepts it. | `main.go` `loadActiveZAK`; `purchase/service.go` field `zak` used in `ComputeMAC` | GW | **Not wired yet** (#121 adds only the parts): `hsm.ZAKSource` and `rotation.ActiveKeys`, reloaded after ACTIVATE. `main.go` still copies the ZAK once into purchase, advtxn and SAF, waiting for #117/#120, which rewrite those constructors. ZAK rotation is disabled meanwhile (SEC-G15) |
| SEC-G11 | A ZPK rotation has no effect on the gateway's transaction path: `hsm.TranslatePIN` has no caller, because the POS sends no PIN block (`docs/plans/MCN-305-pos-canvas.md` Ruling R1; R5.2). Only the issuer's PVV check uses the ZPK. | `grep TranslatePIN` → only `internal/hsm` | GW | MCN-502 follow-up when PIN entry returns |
| SEC-G12 | The key-change message differs from docs/03 §11 ("key index in DE 53"): the gateway sends `"<ZPK|ZAK>:" + hex` in DE 48 and no DE 53 (MCN-504-ISS ruling, PR #60) | `runner.go` `runSend0800161` comment | PLAT (docs) | Amend docs/03 §7.1 / §11 to the implemented DE 48 layout, or add DE 53 via a contracts PR |
| SEC-G13 | The real inventory has 2 rows (ZAK, ZPK) with `counterparty: null`, against the canvas's six with `issuer`. The acquirer `key_store` can't hold CVK/PVK (CHECK), and the ZMK isn't stored (it's config). | live §4.1; `00004_key_store.sql` | GW | Register the ZMK (KCV only) at startup; CVK/PVK belong to the issuer inventory (SEC-G1) |
| SEC-G14 | Rotation audit rows have no actor (the gateway `audit_log` has no actor column; NFR-07 requires one), and there's no dual control, although the PCI list says "cần hai người duyệt" / "dual control" | `00005_key_rotation.sql` `audit_log`; `vi.json` `security.pci.items.audit` | GW + WEB | Add `actor` (from `X-Actor`); either implement a second approval or change the copy |
| SEC-G15 | The issuer never used a rotated ZAK/ZPK: `VerifySecurity` and `Respond` read `ZAK_HEX`/`ZPK_HEX` from env once, and only the retired-key fallback read `key_store`, so a ZAK rotation turned into an outage (RC 96 on every MAC once the 5-minute window closed) and a ZPK rotation broke PIN verification | `VerifySecurity.setConfiguration`, `Respond.setConfiguration` | ISS | **Fixed** in #122: `VerifySecurity` (ZAK and ZPK) and `Respond` (ZAK) read the ACTIVE key from `key_store` on every message through the `SessionKeys` port (`KeyStoreSessionKeys`), so the key `ReceiveKeyChange` activates is used from the next message. The environment key only seeds `key_store` when no key is ACTIVE (at most one ACTIVE per type: migration V8). The key the last change retired is still accepted for 5 minutes, and a key that was never activated never is. Keys are at rest only as LMK cryptograms; each clear copy is zeroed after its one use. `ReceiveKeyChange` is idempotent: a resent 0800/161 carrying the key that is already ACTIVE (the gateway resends an unanswered one, #121) answers 0810 00 with no new row, so the retired key stays the one before the change. The same ACTIVE ZAK now MACs every 0430 as well (network-page.md NET-G19). The gateway's 409 guard on ZAK rotations (#121) can be lifted once this lands |
| SEC-G16 | A key-change 0800/161 that gets no 0810 within any of its attempts (each bounded by `ECHO_TIMEOUT`) leaves the rotation `FAILED` with an unknown outcome. The issuer may or may not have activated the key, so the gateway keeps it PENDING, and the MAC verifier retries a response under it before the recently retired key (#121). A later successful rotation of the same type retires it; otherwise nothing settles it. | `internal/rotation/runner.go` `runSend0800161`; `store.MACFallbackKeys` | GW + ISS + contracts | A key-status query (e.g. 0800 with a KCV check) to resolve the outcome, then activate or retire the PENDING key |
| SEC-G17 | The issuer's `ReceiveKeyChange` is not idempotent: a resent 0800/161 with the same cryptogram (the gateway resends after a lost 0810, SEC-G16) inserts and activates another `key_store` row and moves the issuer's recently-retired key, so the dual-key window can end up pointing at the same key twice. | `issuer-jpos` `ReceiveKeyChange`; `gateway-go/internal/rotation/runner.go` `runSend0800161` | ISS | **Fixed** in #122: a key-change advice whose unwrapped key equals the ACTIVE key (compared in constant time, not by KCV) answers 0810 00 with no insert, no activation and no audit row, so a resend (same cryptogram, or the same key under a fresh nonce) never shifts the recently retired key |
| SEC-G18 | A replayed key-change advice could bring back an old key: a cryptogram of a key that a later rotation had already retired, sent again, was inserted and activated like a new key | `issuer-jpos` `ReceiveKeyChange` | ISS | **Fixed** in #122: a key change whose unwrapped key equals any RETIRED key of that type and counterparty (constant-time comparison) is declined 0810 RC 96, with no insert and no activation, and writes an `audit_log` row `key_change.replay_rejected` (key type, counterparty and the matching row's id; no key material) |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version, verified against main @ `8d27c72` and the local stack (GET only). |
| 1.1 | 2026-09-25 | SEC-G2–G6, G8, G9 fixed; SEC-G10 port and adapter (#121) |
| 1.2 | 2026-09-26 | #121 review: ZAK rotation disabled (SEC-G15), unknown-outcome rotations keep their key (SEC-G16), SEC-G8/G10 rows made accurate |
| 1.3 | 2026-09-26 | #121 re-review: bounded key-change attempts, PENDING-then-retired MAC fallback, stale PENDING keys retired on a later activation (SEC-G16); SEC-G17 added |
| 1.4 | 2026-09-26 | SEC-G1 fixed; SEC-G8 web part (rows keyed by type + KCV, rotate only on the active ZPK) (#124) |
| 1.5 | 2026-09-26 | SEC-G15 (issuer never used a rotated ZAK/ZPK, including the 0430 MAC), SEC-G17 (idempotent key-change advice) and SEC-G18 (replayed old key rejected) marked Fixed in #122. |
