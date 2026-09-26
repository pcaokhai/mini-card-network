# Network operations page: API contract

| | |
| --- | --- |
| Document | `docs/api/network-page.md` |
| Version | 1.5 |
| Status | Approved for integration (links, events, SAF, echo); Draft for `/v1/network/switch` and `/v1/terminals`, which the provider doesn't have yet |
| Date | 2026-09-25 |
| Screen | route `/network`, container `web-next/src/app/(console)/network/NetworkScreen.tsx` |
| Stories | MCN-205 (WEB), MCN-804 (WEB, switch/STIP parts), MCN-204 (GW), MCN-802 (GW, circuit breaker and STIP, not built), MCN-404 (GW, `ISSUER_DOWN` scenario) |
| Provider(s) | gateway-go (`internal/api/network.go`, `internal/isonet/supervisor.go`, `internal/store/links.go`, `internal/store/safqueue.go`, `internal/api/chaos.go`, `internal/ws/hub.go`) |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/network-client.ts` and `src/shared/api/chaos-client.ts`; WS through `src/shared/ws/useWsEvents.ts` |
| Design reference | canvas `project/Network.dc.html` (docs/01-prd.md §7.1); plan `docs/plans/MCN-205-network-canvas.md`; release notes `docs/releases/R5.3.md` "Vận hành mạng" |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only). Echo, sign-on, sign-off and the chaos PUT were **not** called on the shared stack; their examples come from dev:mock (`web-next/src/mocks/pages/network.ts`) or from the provider code, labelled as such |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and are not repeated.

## 1. Purpose and scope

The Network operations page shows an operator whether the card network is healthy: the POS → acquirer → switch → issuer topology, the ISO 8583 links with their status, latency and last echo, the circuit breaker and stand-in (STIP) state, the store-and-forward (SAF) queue, and today's network events. It offers two actions: a per-link manual echo ("Kiểm tra ngay") and a page-level "Mô phỏng ngân hàng phát hành sập" toggle, which drives the `ISSUER_DOWN` chaos scenario.

This contract covers every call and WebSocket event the page consumes. It also documents `POST …/sign-on` and `POST …/sign-off`. The client supports them, but the page renders no button for them (plan Ruling 2).

Out of scope: the other five chaos scenarios and chaos runs (see [chaos-lab-page.md](chaos-lab-page.md)), and the header link pill, which is a shared component that reads the same `GET /v1/network/links`.

## 2. Page map

| UI region (canvas / i18n label) | Data shown | Call(s) (§4.x) | Refresh |
| --- | --- | --- | --- |
| Title "Vận hành mạng" + subtitle | static copy | none | none |
| Toggle "Mô phỏng ngân hàng phát hành sập" / "Khôi phục ngân hàng phát hành" (`network.toggle.*`) | label follows `ISSUER_DOWN.enabled`; disabled until scenarios load and while the PUT is pending | §4.9, §4.10 | once on mount; after each toggle, every `["network", …]` query is invalidated |
| "Sơ đồ kết nối": node "Máy POS" | Easy "{count} máy đã đăng ký" / Expert "{count} terminals registered"; unavailable: "Chưa có danh sách máy" / "GET /v1/terminals · chưa có" | §4.8 | once on mount |
| "Sơ đồ kết nối": node "Ngân hàng thanh toán" | Easy "{count} máy chủ gateway" / Expert "{names} (Go)"; down: "Mất kết nối tới mạng" / "{names} DOWN" | §4.1 `from`, `status` | 5 s poll + WS |
| "Sơ đồ kết nối": node "Bộ chuyển mạch" | Easy "Định tuyến theo đầu số thẻ" / "Đang duyệt thay"; Expert "BIN routing · MUX pool" / "STIP ACTIVE · circuit {circuit}"; unavailable: "Chưa có dữ liệu" / "GET /v1/network/switch · chưa có" | §4.7 | 5 s poll + WS |
| "Sơ đồ kết nối": node "Ngân hàng phát hành" | Easy "{count} máy chủ đang chạy" / Expert "{names} (jPOS)"; down: "Không phản hồi" / "{names} DOWN" | §4.1 `to`, `status` | 5 s poll + WS |
| Topology segments (flowing when up, red dashed when down) | POS→acquirer always up; acquirer→switch and switch→issuer from link status | §4.1 | 5 s poll + WS |
| "Các đường kết nối" table | one row per link: name, status badge, "Độ trễ" / "p99", "Kiểm tra gần nhất" / "Echo 0800/301", button "Kiểm tra ngay" / "Gửi echo" | §4.1, §4.2 | 5 s poll + WS; the age ticks every 1 s on the client |
| "Nhật ký sự kiện hôm nay" | today's events, newest first, 5 visible, "Xem thêm {count} sự kiện" (max 30) | §4.6 | 5 s poll + WS |
| "Tự ngắt và duyệt thay" / "Circuit breaker và STIP" | pills "Bình thường" / "Đã ngắt" / "Đang thử lại" (Expert: CLOSED / OPEN / HALF_OPEN), description, tiles "Duyệt thay tối đa mỗi giao dịch" and "Đã duyệt thay" | §4.7 | 5 s poll + WS |
| "Hàng đợi gửi lại" / "SAF queue" | badge "{count} lệnh" / "Trống", up to 30 items (title, "RRN {rrn}", "Đã thử {n} lần" / "attempts {n}") | §4.5 | 5 s poll + WS |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency (If-Match/ETag) | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/network/links` | gateway-go | ISO link status and echo state | on mount, every 5 s, on WS `link.status`, after each link action | n/a | none | Real |
| 4.2 | `POST /v1/network/links/{linkId}/echo` | gateway-go | Send 0800/301 now | click "Kiểm tra ngay" / "Gửi echo" | sent (new UUID per click); not checked, not deduplicated | none | Real |
| 4.3 | `POST /v1/network/links/{linkId}/sign-on` | gateway-go | Send 0800/001 now | never from this page (Ruling 2); `useLinkAction` supports it | sent; not checked | none | Real (no UI) |
| 4.4 | `POST /v1/network/links/{linkId}/sign-off` | gateway-go | Send 0800/002 now | never from this page (Ruling 2) | sent; not checked | none | Real (no UI), **destructive** |
| 4.5 | `GET /v1/network/saf` | gateway-go | SAF depth, dead count and open items | on mount, every 5 s, on WS `saf.changed` | n/a | none | Real |
| 4.6 | `GET /v1/network/events` | gateway-go | Operational event log | on mount, every 5 s, on WS `network.event` | n/a | none | Real (`limit` and `cursor` ignored) |
| 4.7 | `GET /v1/network/switch` | gateway-go | Circuit breaker and STIP status | on mount, every 5 s, on WS `switch.status` | n/a | none | **Planned MCN-802** (real gateway: 404); dev:mock only |
| 4.8 | `GET /v1/terminals` | gateway-go | Registered terminals (count for the POS node) | once on mount | n/a | none | **Not built** (real gateway: 404); dev:mock only |
| 4.9 | `GET /v1/chaos/scenarios` | gateway-go | Reads `ISSUER_DOWN.enabled` for the toggle label | once on mount; after a toggle | n/a | none | Real |
| 4.10 | `PUT /v1/chaos/scenarios/ISSUER_DOWN` | gateway-go | Simulate issuer down / restore | click the toggle | required (new UUID per click); presence checked only | none | Real, **destructive** on a shared stack |
| 5.1 | WS `link.status` | gateway-go | Link status changed | push | – | – | Real (partial payload) |
| 5.2 | WS `network.event` | gateway-go | Event appended | push | – | – | Real (id and time not set) |
| 5.3 | WS `saf.changed` | gateway-go | SAF depth changed | push | – | – | **Never emitted** |
| 5.4 | WS `switch.status` | gateway-go | Breaker state changed | push | – | – | **Never emitted** (MCN-802) |
| – | Client clock | web-next | echo ages, the "today" filter | 1 s tick | – | – | Real |

## 4. Calls

### 4.1 GET /v1/network/links

- **Summary.** Lists the ISO links. v1 has exactly one: gateway → issuer.

**Request.** No parameters, no body.

**Response.** `200 OK`, array of `Link`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `linkId` | string | ✓ | row key; echo target | not shown | not shown |
| `from` | string | ✓ | row name; acquirer node instances | "Gateway → Issuer" (id title-cased per `-` part), sub-line `gateway → issuer` | "gateway → issuer", sub-line "ISO 8583 · 2-byte header"; node "{names} (Go)" |
| `to` | string | ✓ | row name; issuer node (links whose `to` starts with `issuer`); switch detection (`to` starts with `switch`) | as above | as above; node "{names} (jPOS)" |
| `status` | `DISCONNECTED` \| `CONNECTED` \| `SIGNED_ON` \| `DOWN` | ✓ | badge, node tone, segment state (only `SIGNED_ON` counts as up) | "Ngắt kết nối" / "Đã kết nối" / "Đã đăng nhập" / "Mất kết nối" | raw enum |
| `lastEchoAt` | date-time or null | ✓ (nullable) | "Kiểm tra gần nhất" / "Echo 0800/301" | "Vừa xong" (< 3 s), "{n} giây trước", "{n} phút trước", "{n} giờ trước"; "Chưa kiểm tra" when null | same |
| `lastEchoOk` | boolean or null | ✓ (nullable) | same cell | `false` → "Không phản hồi" | same |
| `p99LatencyMs` | integer or null | ✓ (nullable) | "Độ trễ" / "p99" | "{ms} ms"; "—" when null or status `DOWN` | same |
| `inFlight` | integer | ✓ | not shown | – | – |

**Provider rules.**
1. The response always holds exactly one element, `linkId: "issuer"`, `from: "gateway"`, `to: "issuer"` (`issuerLinkID` in `internal/api/network.go`; `from` is the constant `gateway` in `internal/store/links.go`).
2. `status` is the row in `link_state` written by the supervisor on every transition: `CONNECTED` after the TCP connect, `SIGNED_ON` after the 0810 to 0800/001 answers RC 00, `DOWN` when the connection ends (`internal/isonet/supervisor.go` `setStatus`). `DISCONNECTED` is never written by the gateway today.
3. `lastEchoAt` moves only on a successful **periodic** echo (every 60 s, `RecordEcho`). A manual echo (§4.2) does not move it (plan Ruling 11).
4. `lastEchoOk` is derived: `true` whenever `lastEchoAt` is set, `null` otherwise. It is never `false`.
5. `p99LatencyMs` is always `null` and `inFlight` always `0`: no code writes those columns.
6. Timestamps are RFC 3339 UTC with microseconds.
7. The ISO link goes down after 3 consecutive failed echoes (docs/03 §9), so `status` can lag a dead issuer by up to 3 minutes. When the issuer rejects a request with RC 91, the gateway signs on again on its own (R5.2 item 12).

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 500 | `link-read-failed` | `link_state` read failed | TanStack keeps the last data, so the table and topology keep their last state; on a first-load failure the table shows "Chưa có đường kết nối nào được báo về." and the acquirer/issuer nodes render as down |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | gateway unreachable | same |

**Example.** Real, local stack, 2026-09-25.

```json
[
  {
    "linkId": "issuer",
    "from": "gateway",
    "to": "issuer",
    "status": "SIGNED_ON",
    "lastEchoAt": "2026-09-25T09:30:59.287786Z",
    "lastEchoOk": true,
    "p99LatencyMs": null,
    "inFlight": 0
  }
]
```

**Notes.** Query key `["network","links"]`, `refetchInterval: 5000`, default retries (3). No `ETag`. Rate limit: not enforced. The header pill on every screen reads the same query.

### 4.2 POST /v1/network/links/{linkId}/echo

- **Summary.** Sends an out-of-band 0800 with DE 70 = 301 on the live connection and reports the result. A down link is reported as `ok: false`, never as an error (MCN-204-AC3).

**Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `linkId` | path | string | ✓ | must be `issuer` | – |

| Header | Required | Notes |
| --- | --- | --- |
| `Idempotency-Key` | contract: yes | the client sends a fresh UUID per click; the provider neither checks nor stores it |
| `traceparent` | no | forwarded by the BFF |

No body.

**Response.** `200 OK`, `EchoResult`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `ok` | boolean | ✓ | not shown; the page refetches links | – | – |
| `latencyMs` | integer or null | – | not shown | – | – |
| `responseCode` | string or null | – | not shown | – | – |

**Provider rules.**
1. No live connection: `{ "ok": false, "latencyMs": null, "responseCode": null }` at once.
2. Send failure or timeout: `ok: false`, `latencyMs` and `responseCode` both null. The timeout is the echo timeout, which defaults to the 60 s echo interval, longer than the server's 35 s `WriteTimeout` (NET-G9).
3. An answer arrives: `ok` is `responseCode == "00"`, and `latencyMs` is the round trip in ms.
4. The manual echo holds the supervisor's trigger lock, so the periodic echo and other triggers wait behind it.
5. It writes no `link_state` change, no `network_event` row and no WS event (NET-G2).

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | `unknown-link` | `linkId` ≠ `issuer` | none visible; the button re-enables and links refetch |
| 500 | `echo-failed` | declared by the handler; `TriggerEcho` never returns an error today | same |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | gateway unreachable | same |

**Example.** Not called on the shared stack. Mock (dev:mock, `mocks/pages/network.ts`):

```json
{ "ok": true, "latencyMs": 38, "responseCode": "00" }
```

Provider rule 1 (link down), from `internal/isonet/supervisor.go`:

```json
{ "ok": false, "latencyMs": null, "responseCode": null }
```

**Notes.** `useLinkAction` makes no optimistic update for echo. The row's button is disabled while that link's mutation is pending, and links are invalidated on settle. No retry (TanStack default for mutations). Rate limit: not enforced.

### 4.3 POST /v1/network/links/{linkId}/sign-on

- **Summary.** Sends 0800/001 on the live connection (docs/03 §7.1). No page button (Ruling 2): the gateway signs on by itself on connect and after RC 91 (R5.2 item 12).

**Request.** As §4.2 (`linkId` = `issuer`, `Idempotency-Key` sent but not checked, no body).

**Response.** `200 OK`, `Link` (§4.1 fields), read back from `link_state` after the 0810.

**Provider rules.**
1. With no live connection: 409 `link-not-ready`.
2. The 0810 must carry RC 00; anything else gives 409 `link-not-ready` with detail `unexpected response code NN`.
3. The link status isn't written, and no event or WS message is produced. The returned `Link` is whatever `link_state` already held, normally `SIGNED_ON`.
4. Client (if a button is ever rendered): an optimistic `status: SIGNED_ON`, rolled back on error.

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | `unknown-link` | `linkId` ≠ `issuer` | optimistic status rolled back |
| 409 | `link-not-ready` | no live connection, send failure/timeout, or RC ≠ 00 | same |
| 500 | `link-read-failed` | reading `link_state` after the 0810 failed | same |

**Example.** Not called on the shared stack. The shape equals the §4.1 element.

### 4.4 POST /v1/network/links/{linkId}/sign-off

- **Summary.** Sends 0800/002 on the live connection. **Destructive on a shared stack**; no page button.

**Request.** As §4.2.

**Response.** `200 OK`, `Link` read back from `link_state`.

**Provider rules.**
1. With no live connection: 409 `link-not-ready`.
2. The 0810's RC is not checked: any answer counts as success. The wait has no timeout of its own; only the HTTP request's context bounds it. A send error or a cancelled request gives 409 `link-not-ready`.
3. The gateway keeps the TCP connection and does **not** change `link_state`, so the response and the next poll still show `SIGNED_ON` (NET-G3). The client's optimistic `DISCONNECTED` flips back on the refetch.
4. The issuer does record the acquirer as signed off (per-acquirer `acquirer_link`). It then answers RC 91 to every 0200/0420 until the gateway signs on again. The gateway notices only when a non-08xx request comes back RC 91, then re-signs on (R5.2 item 12). Echoes are answered whatever the sign-on state, so the periodic echo never notices. The request that hit RC 91 stays failed; per R5.2 it surfaced as a decline and a queued reversal.
5. Because the issuer's sign-on state is per acquirer ID, not per connection, a sign-off from any session affects every user of the stack.

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | `unknown-link` | `linkId` ≠ `issuer` | optimistic status rolled back |
| 409 | `link-not-ready` | no live connection or send failure | same |
| 500 | `link-read-failed` | reading `link_state` failed | same |

**Example.** Not called on the shared stack. Response shape: the §4.1 element, with `status` still `SIGNED_ON` (provider rule 3).

### 4.5 GET /v1/network/saf

- **Summary.** Store-and-forward queue: open items and counts.

**Request.** No parameters.

**Response.** `200 OK`, inline `{ depth, deadCount, items: SafItem[] }`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `depth` | integer ≥ 0 | ✓ | badge; empty text | "{count} lệnh" or "Trống"; empty "Không có lệnh nào đang chờ. Mọi thông báo đã tới nơi." | same badge; empty "depth {depth} · dead {dead} · oldest —" |
| `deadCount` | integer ≥ 0 | ✓ | badge tone (red when > 0, amber when depth > 0) | colour only | also in the empty line |
| `items[].id` | string | ✓ | row key | – | – |
| `items[].mti` | `0120` \| `0220` \| `0420` | ✓ | row title | "Lệnh hủy" (0420) or "Thông báo duyệt thay" | "{mti} reversal" or "{mti} advice · STIP approved" |
| `items[].amount` | Money | – | row title suffix | "{label} · 450.000 ₫" | not shown |
| `items[].rrn` | string | ✓ | "RRN {rrn}" | shown | shown |
| `items[].attempts` | integer | ✓ | right column | "Đã thử {n} lần" | "attempts {n}" |
| `items[].status` | `PENDING` \| `IN_FLIGHT` \| `ACKED` \| `DEAD` | ✓ | row `data-status` (styling) | – | – |
| `items[].nextRetryAt` | date-time | ✓ | not shown | – | – |
| `items[].lastError` | string or null | – | not shown | – | – |

**Provider rules.**
1. `items` holds every `saf_queue` row in `PENDING`, `IN_FLIGHT` or `DEAD`, oldest first (`ORDER BY id`). `ACKED` rows are never returned.
2. `depth = items.length`, so it **includes** `DEAD` rows. `deadCount` counts the `DEAD` rows among them.
3. `amount` is the original transaction's amount in minor units (`tran_log.amount`), currency ISO 4217 numeric.
4. The list is not paginated or capped (NET-G7). The UI renders the first 30.
5. `nextRetryAt` is RFC 3339 UTC, second precision.

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 500 | `saf-read-failed` | query failed | the card keeps its last data; on a first-load failure it shows "Trống" with depth 0 (misleading, NET-G11) |
| 502 | BFF `upstream-unavailable` | gateway unreachable | same |

**Example.** Real, local stack, 2026-09-25 (queue empty):

```json
{ "depth": 0, "deadCount": 0, "items": [] }
```

Mock (dev:mock, issuer down), trimmed:

```json
{
  "depth": 37,
  "deadCount": 0,
  "items": [
    { "id": "saf-3", "mti": "0420", "rrn": "626514000199", "amount": { "amount": 450000, "currency": "704" }, "attempts": 6, "status": "PENDING", "nextRetryAt": "2026-09-25T09:31:30.000Z", "lastError": "issuer timeout" },
    "…"
  ]
}
```

**Notes.** Query key `["network","saf"]`, 5 s poll. The Chaos Lab reads the same query (for `depth`). No `ETag`. Rate limit: not enforced.

### 4.6 GET /v1/network/events

- **Summary.** The most recent operational events, newest first.

**Request.**

| Parameter | In | Type | Required | Constraints | Default |
| --- | --- | --- | --- | --- | --- |
| `limit` | query | integer | no | contract 1–200 | 50; **ignored by the provider** |
| `cursor` | query | string | no | opaque | **ignored by the provider** |

The client sends neither.

**Response.** `200 OK`, inline `{ items: NetworkEvent[], nextCursor }`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `items[].id` | string | ✓ | list key | – | – |
| `items[].occurredAt` | date-time | ✓ | time column; "today" filter; sort | `HH:mm:ss` (vi-VN, 24 h, local time zone) | same |
| `items[].severity` | `INFO` \| `OK` \| `WARN` \| `ERROR` | ✓ | dot tone (info / ok / warn / bad) | colour | colour |
| `items[].easyText` | string (English from the gateway) | ✓ | event text | mapped to Vietnamese when it is one of the five known texts (below); otherwise shown as sent | not shown |
| `items[].technicalText` | string | ✓ | event text | not shown | shown as sent |
| `nextCursor` | string or null | ✓ | not read | – | – |

The five known gateway texts (plan Ruling 7, `gatewayEventKey` in `src/components/network/network-model.ts`):

| Gateway `easyText` | Severity | Written by | Easy text (`network.events.known.*`) |
| --- | --- | --- | --- |
| `Link to issuer is up` | INFO | supervisor, after sign-on | "Đường kết nối tới ngân hàng phát hành đã hoạt động" |
| `Link to issuer is down` | WARN | supervisor, connection lost | "Mất kết nối tới ngân hàng phát hành, đang thử kết nối lại" |
| `Signed on again: the issuer had the link signed off` | WARN | supervisor, re-sign-on after RC 91 | "Đã đăng nhập lại: ngân hàng phát hành đã đăng xuất đường kết nối" |
| `Issuer still holds the link signed off` | WARN | supervisor, re-sign-on after RC 91 failed | "Ngân hàng phát hành vẫn đang giữ đường kết nối ở trạng thái đăng xuất" |
| `A response arrived too late for a transaction` | WARN | purchase service, late 0210 | "Có câu trả lời đến quá muộn cho một giao dịch" |

**Provider rules.**
1. Returns the 50 newest `network_event` rows, `ORDER BY occurred_at DESC`. `nextCursor` is always `null`.
2. `id` is the row's `BIGINT` as a decimal string.
3. The late-response event is persisted through `LinkRepository.RecordEvent` before it is broadcast, so it appears in this list.
4. The gateway never emits severity `OK` or `ERROR` today.
5. Client derivation: keep events on the viewer's **local** calendar day, sort newest first, cap at 30, show 5 with "Xem thêm {count} sự kiện". Empty: "Hôm nay chưa có sự kiện mạng nào."

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 500 | `events-read-failed` | query failed | last data kept; on a first-load failure the empty text shows |
| 502 | BFF `upstream-unavailable` | gateway unreachable | same |

**Example.** Real, local stack, 2026-09-25, trimmed to 3 of 16 items (`?limit=2` also returned 16):

```json
{
  "items": [
    { "id": "16", "occurredAt": "2026-09-25T07:57:59.246361Z", "severity": "INFO", "easyText": "Link to issuer is up", "technicalText": "signed on" },
    { "id": "15", "occurredAt": "2026-09-25T07:50:22.649492Z", "severity": "INFO", "easyText": "Link to issuer is up", "technicalText": "signed on" },
    { "id": "14", "occurredAt": "2026-09-25T07:50:22.093099Z", "severity": "WARN", "easyText": "Link to issuer is down", "technicalText": "reconnecting with backoff" },
    "…"
  ],
  "nextCursor": null
}
```

**Notes.** Query key `["network","events"]`, 5 s poll. Rate limit: not enforced.

### 4.7 GET /v1/network/switch

- **Summary.** Circuit breaker and STIP status of the switch (MCN-802). **Not built**: the real gateway answers `404 page not found` (plain text).

**Request.** No parameters.

**Response (contract).** `200 OK`, `SwitchStatus`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `circuit` | `CLOSED` \| `OPEN` \| `HALF_OPEN` | ✓ | current pill; description; switch node | "Bình thường" / "Đã ngắt" / "Đang thử lại" + `network.breaker.desc.{circuit}.easy` | raw enum + `…tech` description |
| `stipActive` | boolean | ✓ | switch node tone (amber when true or circuit ≠ CLOSED) | "Đang duyệt thay" | "STIP ACTIVE · circuit {circuit}" |
| `stipLimit` | Money | ✓ | tile "Duyệt thay tối đa mỗi giao dịch" / "STIP limit / txn" | "500.000 ₫" | same |
| `stipApprovedCount` | integer | ✓ | tile "Đã duyệt thay" / "STIP approved" | vi-VN grouping | same |

**Provider rules (for MCN-802).** 1. The breaker thresholds follow docs/02 §7.4. 2. `switch.status` is emitted on every state change (MCN-802-AC1).

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | none (plain-text `404 page not found`, route not mounted) | always, until MCN-802 | switch node "Chưa có dữ liệu" / "GET /v1/network/switch · chưa có"; breaker "Bộ chuyển mạch chưa báo trạng thái…" / "GET /v1/network/switch chưa có trên gateway (MCN-802)…"; tiles "—" (plan Ruling 5) |

**Example.** Real: `404 page not found`. Mock (dev:mock, issuer down):

```json
{ "circuit": "OPEN", "stipActive": true, "stipLimit": { "amount": 500000, "currency": "704" }, "stipApprovedCount": 37 }
```

**Notes.** 5 s poll, and TanStack retries each failure 3 times, so today the page makes up to 4 failing requests per poll cycle against the real gateway (NET-G6).

### 4.8 GET /v1/terminals

- **Summary.** Registered terminals, counted on the POS node. **Not built** in gateway-go: the real gateway answers `404 page not found`.

**Request.** No parameters. **Response (contract).** `200 OK`, array of `Terminal`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| array length | – | – | POS node | "{count} máy đã đăng ký" | "{count} terminals registered" |
| `terminalId`, `merchantId`, `merchantName`, `mcc` | string | ✓ | not shown | – | – |

**Provider rules (proposed).** 1. It returns every terminal in the acquirer's `terminal` table. The API has no online flag, so the page doesn't claim "đang trực tuyến" (Ruling 4).

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 404 | none (plain text, route not mounted) | always today | POS node "Chưa có danh sách máy" / "GET /v1/terminals · chưa có" |

**Example.** Real: `404 page not found`. Mock (dev:mock), 1 of 42:

```json
[{ "terminalId": "POS00001", "merchantId": "MCN000000000001", "merchantName": "Cà phê Góc Phố", "mcc": "5814" }, "…"]
```

**Notes.** Query key `["network","terminals"]`, no polling.

### 4.9 GET /v1/chaos/scenarios

- **Summary.** Reads the six chaos scenarios. This page uses only `ISSUER_DOWN.enabled`. Full field table: [chaos-lab-page.md](chaos-lab-page.md) §4.1.

| Field | UI element on this page | Easy / Expert |
| --- | --- | --- |
| `[id=ISSUER_DOWN].enabled` | toggle label | `false` → "Mô phỏng ngân hàng phát hành sập"; `true` → "Khôi phục ngân hàng phát hành" (both modes) |

**Errors.** As in chaos-lab-page.md §4.1 (`chaos-list-failed` 500 when Toxiproxy is unreachable). The toggle stays disabled until the query succeeds.

**Example.** Real, local stack, 2026-09-25, trimmed:

```json
[ "…", { "id": "ISSUER_DOWN", "enabled": false, "easyText": "The issuer is completely unreachable.", "technicalText": "reset_peer toxic on the whole issuer link." }, "…" ]
```

### 4.10 PUT /v1/chaos/scenarios/ISSUER_DOWN

- **Summary.** Turns the `ISSUER_DOWN` scenario on or off (MCN-804-AC2). **Destructive on a shared stack**: it adds a Toxiproxy `reset_peer` toxic to the issuer proxy, which cuts the issuer link for every user. Full contract: [chaos-lab-page.md](chaos-lab-page.md) §4.2.

**Request.** Path `scenarioId = ISSUER_DOWN`; header `Idempotency-Key` (fresh UUID per click, required); body `{ "enabled": true | false }`, the negation of the current state.

**Response.** `200 OK`, `ChaosScenario` for `ISSUER_DOWN`. The page doesn't read the body; on settle it invalidates the scenarios query and every `["network", …]` query.

**What the page shows afterwards (real stack).** The link drops to `DOWN` on the next message through the proxy (the periodic echo within 60 s, or a purchase), then cycles `CONNECTED` → `DOWN` as each reconnect's sign-on is reset. Events show "Mất kết nối tới ngân hàng phát hành, đang thử kết nối lại". No breaker or STIP change occurs, because MCN-802 isn't built. Restoring removes the toxic; the supervisor reconnects within its backoff (1 s to 30 s) and logs "Đường kết nối tới ngân hàng phát hành đã hoạt động".

**Errors.** As in chaos-lab-page.md §4.2 (`idempotency-key-required`, `invalid-request`, `chaos-set-failed`, `chaos-list-failed`). **No error is shown on this page** (NET-G10): the button re-enables and the label keeps the unchanged state.

**Example.** Not called on the shared stack. Mock (dev:mock, `mocks/pages/network.ts`):

```json
{ "id": "ISSUER_DOWN", "enabled": true, "easyText": "", "technicalText": "ISSUER_DOWN" }
```

## 5. Real-time events

Socket: `NEXT_PUBLIC_WS_URL` (set in the local `.env.local` to `ws://localhost:8080/v1/stream`, direct to the gateway), else same-origin `/v1/stream`. The BFF doesn't proxy WebSockets (NET-G8). The envelope is `{ id, type, occurredAt, data }`; the gateway sends no `traceId`. The client reconnects with backoff from 1 s to 30 s. It doesn't validate `data`: every handled event only **invalidates** the matching query, so the REST call is the source of truth and the payload is ignored. The 5 s poll remains as a backstop (MCN-205-AC3).

| Event | `data` schema | Emitted by (real) | UI effect | Ordering / dedupe |
| --- | --- | --- | --- | --- |
| `link.status` | `Link` | `Supervisor.setStatus` on CONNECTED, SIGNED_ON and DOWN transitions | invalidates `["network","links"]` | none needed (refetch). Payload is partial: only `linkId`, `from`, `to`, `status` are real; `lastEchoAt`, `lastEchoOk`, `p99LatencyMs` are null and `inFlight` is 0 (NET-G4) |
| `network.event` | `NetworkEvent` | `Supervisor.recordEvent` (4 texts); `purchase.Service.RecordLateResponse` (late 0210) | invalidates `["network","events"]` | none. Payload `id` is `"0"` and `occurredAt` is `0001-01-01T00:00:00Z` (not the stored row, NET-G4). A late-response event is persisted first, so the refetch contains it |
| `saf.changed` | `{ depth, deadCount }` | **never emitted** | would invalidate `["network","saf"]` | the 5 s poll is the only update path |
| `switch.status` | `SwitchStatus` | **never emitted** (MCN-802) | would invalidate `["network","switch"]` | – |

`heartbeat` (docs/04 §5) is not sent by the gateway and not consumed by the page.

## 6. Security and compliance

| Topic | Rule on this page |
| --- | --- |
| Card data | No response on this page carries a PAN, track data, PIN block or key. SAF items carry RRN and amount only. |
| Key material | None. |
| Destructive actions | Sign-off (§4.4) and the `ISSUER_DOWN` toggle (§4.10) disrupt every user of a shared stack: the issuer's sign-on state is per acquirer ID, and the toxic cuts the one issuer proxy. Safeguards today: sign-on and sign-off have no button (Ruling 2); the toggle has no confirmation and no role check (v1 has no end-user auth). Treat both as lab-only operations. |
| Idempotency | The link actions ignore `Idempotency-Key`. A retried sign-off or echo is resent as a new 0800 with a new STAN. That is harmless for echo and sign-on; sign-off repeats the disruption. |
| Audit trail | Manual echo, sign-on and sign-off write no `network_event` and no audit record. The chaos toggle is not audited either. The BFF forwards no `X-Actor` (docs/04 §2). |
| Logs | The link and event paths log no card data. |

## 7. Non-functional requirements

| Call | Latency budget | Observed on the local stack (2026-09-25, 50 GETs direct to :8080) | Payload bound | Pagination |
| --- | --- | --- | --- | --- |
| §4.1 links | no PRD NFR; UI target < 300 ms | p50 1.3 ms, p99 3.4 ms (observed) | 1 item, ≈ 190 B | none |
| §4.2 echo | the issuer answers an echo within its processing target (p99 < 100 ms, docs/03 §9) | not called (shared stack) | 60 B | – |
| §4.3 / §4.4 sign-on / sign-off | as echo; sign-on is bounded by the echo timeout (60 s), sign-off only by the request context | not called | ≈ 190 B | – |
| §4.5 SAF | < 300 ms | p50 1.5 ms, p99 3.9 ms (observed, empty queue) | **unbounded** (NET-G7) | none |
| §4.6 events | < 300 ms | p50 2.6 ms, p99 4.0 ms (observed) | ≤ 50 items, 2.3 KB at 16 items | contract cursor; provider fixed 50 |
| §4.7 switch / §4.8 terminals | – | 404 in < 4 ms | – | – |

Polling load per open page: 4 requests every 5 s (links, SAF, events, switch; the header's links query shares the cache) ≈ 0.8 req/s. Against today's real gateway, the switch 404s are retried 3 times each (§4.7 Notes). Link status can lag a dead issuer by up to 3 × 60 s (docs/03 §9).

## 8. UI states

| State | What renders | Driving call |
| --- | --- | --- |
| Loading | every card renders at once with empty data: "Chưa có đường kết nối nào được báo về.", "Hôm nay chưa có sự kiện mạng nào.", SAF "Trống", breaker "unavailable" copy, POS "Chưa có danh sách máy"; the toggle is disabled | all queries pending |
| Empty | no links: table empty text, acquirer and issuer nodes red; no events today: empty log text; empty SAF: "Trống" + "Không có lệnh nào đang chờ. Mọi thông báo đã tới nơi." | §4.1, §4.6, §4.5 |
| Partial | each card fails on its own; a failed query keeps its last data. The switch and terminals 404s are the normal partial state on the real stack | any |
| Error | no error text anywhere on the page; failures look like the empty state (NET-G11) | any |
| Provider not available | BFF 502 on every call: same as Loading/Empty, and the header pill reads down | all |
| Action pending | echo: that row's button disabled; toggle: disabled while its PUT is pending | §4.2, §4.10 |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| NET-G1 | A manual echo doesn't move `lastEchoAt`, so "Kiểm tra ngay" gives no visible feedback on the real stack. `EchoResult` is discarded by the UI. | `internal/isonet/supervisor.go:102` `TriggerEcho` never calls `RecordEcho`; plan Ruling 11; R5.3 "Still open" | GW (+ WEB) | **Fixed** in #121: a manual echo calls `RecordEcho` and emits `link.status` + `network.event` (`ECHO_OK`/`ECHO_FAILED`) |
| NET-G2 | MCN-204-AC2 says WS emits `link.status` and `network.event` "on every change". Echo results and manual sign-on/off emit nothing. docs/04 §5 lists "echo result" as a `link.status` trigger. | `internal/api/network.go` handlers; `supervisor.go` only broadcasts from `setStatus`/`recordEvent` | GW | **Fixed** in #121: echo, sign-on and sign-off each record a `network_event` and broadcast it with `link.status` |
| NET-G3 | Sign-off leaves `link_state.status = SIGNED_ON` while the issuer holds the acquirer signed off. The UI shows healthy until a request returns RC 91. Sign-on doesn't write status either. | `supervisor.go:131` `TriggerSignOff` → `signOff` (no `setStatus`); R5.2 item 12 | GW | **Fixed** in #121: sign-off sets `CONNECTED` (the enum has no SIGNED_OFF; the connection stays up), sign-on and the re-sign-on after RC 91 set `SIGNED_ON` |
| NET-G4 | WS payloads aren't the stored resources. `link.status` carries only endpoint and status (nulls elsewhere); `network.event` has `id: "0"` and a zero `occurredAt`. The page is unaffected (it only invalidates), but any consumer that reads `data` gets wrong values. | `supervisor.go:193`, `:201` | GW | **Fixed** in #121: `network.event` is the stored row (`RETURNING id, occurred_at`); `link.status` is the Link read back with live metrics |
| NET-G5 | ~~The late-response event is broadcast but never persisted, so it isn't in `GET /v1/network/events` and vanishes on the refetch the WS event triggers.~~ | **Fixed** (#117): `purchase.Service.RecordLateResponse` persists the event before broadcasting it | GW | Done |
| NET-G6 | `GET /v1/network/switch` isn't built (404, plain text). The page polls it every 5 s, and each failure is retried 3 times. | curl `:8080/v1/network/switch` → 404; not mounted in `MountNetwork` | GW (MCN-802) + WEB | MCN-802; until then, `retry: false` on 404 in `useSwitchStatus` |
| NET-G7 | `GET /v1/network/saf` is unbounded, and `depth` includes DEAD rows. The contract doesn't say whether it should. | `internal/store/safqueue.go:201` | GW + contracts | **Fixed** in #121: `depth` = PENDING + IN_FLIGHT (contract #111), `items` capped at 200, owed advices first |
| NET-G8 | The WebSocket bypasses the BFF: it relies on the uncommitted `NEXT_PUBLIC_WS_URL`, and the same-origin fallback `/v1/stream` isn't served by Next. | `src/shared/ws/useWsEvents.ts`; `web-next/.env.local` | WEB | **Fixed** in #124 (documented): Route Handlers can't hold a WebSocket open, so `web-next/.env.example` documents `NEXT_PUBLIC_WS_URL` (with the BFF upstreams); a same-origin `/v1/stream` needs a reverse proxy |
| NET-G9 | Manual echo and sign-on wait up to the echo timeout (60 s = echo interval), and sign-off has no timeout at all. Both exceed the HTTP `WriteTimeout` (35 s), so the caller can get a dropped connection while the trigger lock still blocks the periodic echo. | `cmd/gateway/main.go:73` (no `EchoTimeout`), `:123`; `supervisor.go:82` | GW | **Fixed** in #121: default `EchoTimeout` capped at 10 s; sign-off bounded by it |
| NET-G10 | A failed `ISSUER_DOWN` PUT shows nothing on this page. The Chaos Lab shows "Không đổi được sự cố. Hãy thử lại." | `NetworkScreen.tsx` `IssuerDownToggle` | WEB | **Fixed** in #124: the same one-line alert as the Chaos Lab |
| NET-G11 | No card distinguishes "failed" from "empty": SAF shows "Trống" and links show "Chưa có đường kết nối nào…" when the gateway is down. | `SafCard.tsx`, `LinksPanel.tsx` | WEB | **Fixed** in #124: the SAF and links cards show a read error from `query.isError` |
| NET-G12 | `GET /v1/network/events` ignores `limit` and `cursor`; `nextCursor` is always null. | `internal/api/network.go` (`defaultEventsLimit`); `?limit=2` returned 16 items | GW | **Fixed** in #121: keyset pagination on `id`; `limit` 1–200 (default 50), bad limit/cursor → 400 `validation-error` |
| NET-G13 | `GET /v1/terminals` isn't implemented anywhere (404), though docs/04 routes it to gateway-go. | curl → 404; no handler in `internal/api` | GW | **Fixed** in #121: `GET /v1/terminals` from `terminal` + `merchant` |
| NET-G14 | Idempotency: the link actions ignore `Idempotency-Key`. The contract marks it required, and docs/04 §2 requires replay for 24 h. | `internal/api/network.go` | GW | **Fixed** in #121: UUID key required on echo/sign-on/sign-off; sign-on/off replay per key (in memory), echo is not replayed (a stale result would mislead) |
| NET-G15 | Gateway event texts are English only, with no language-neutral code. The web maps five exact strings, so any wording change silently falls back to English. | `network-model.ts` `GATEWAY_EVENT_KEYS`; Ruling 7 | contracts + GW | **Fixed**: provider in #121 (`code` on every supervisor event), web in #125 (copy keyed by `code`, no text matching; a codeless older row shows the provider text). The late-response event's `LATE_RESPONSE` code follows #117 |
| NET-G16 | `p99LatencyMs` is always null and `inFlight` always 0: nothing writes those columns. The "Độ trễ" column is always "—" on the real stack. | `migrations/00001_link_state_and_network_event.sql`; no writer in `internal/` | GW | **Fixed** in #121: `p99LatencyMs` over the last 1000 echo/request round trips and `inFlight` from the MUX, computed live rather than stored |
| NET-G17 | Problem `type` values are bare slugs (`unknown-link`, `link-not-ready`), not docs/04 URIs, and `link-not-ready` isn't in the docs/04 §3 catalogue (the nearest is `link-down` 503). | `internal/api/lab.go` `problem()`; `network.go:162` | GW | Shared URI problem writer; align with the catalogue |
| NET-G18 | A manual sign-off does not last: the next request that gets RC 91 makes the supervisor sign on again automatically (`resignOn`), so the operator's choice is silently undone. | `internal/isonet/supervisor.go` `Send` → `signOnAgain` | GW | Remember a manual sign-off and suppress the automatic re-sign-on until a manual sign-on (or reconnect) |
| NET-G19 | The issuer's 0430 (reversal acknowledgement) carried no MAC of its own: `RespondReversal` cloned the 0420 and sent it back with the acquirer's own DE 128 MAC echoed, although docs/03 §4 makes 64/128 mandatory on the 0430 | `RespondReversal.buildResponse` | ISS | **Fixed** in #122: every 0430 from the reversal chain (00, and the 96 that makes the SAF repeat) is MACed under the ACTIVE ZAK from `key_store`, in DE 128 because the echoed DE 90 sets the secondary bitmap (the gateway's own rule for its 0420, MCN-502 Ruling 2), through the shared `ResponseMac` Respond also uses. The gateway ignores the 0430's MAC (`saf/worker.go` checks only DE 39), so this can't break delivery. The two 0430s `ReversalListener` answers itself are MACed through the same `ResponseMac` too: the 91 on a link that isn't signed on (the key store doesn't depend on the link, so the active ZAK is always there), and the fallback when an advice can't be queued, which now says 96 (not recorded, repeat) instead of echoing the 0420's reason code. If signing itself fails (key store unreachable), that 0430 still goes out unsigned rather than not at all |
| NET-G20 | The gateway never verifies the 0430's MAC: the SAF worker marks an advice ACKED on DE 39 = "00" alone, so a forged or corrupted 0430 would complete a reversal the issuer never recorded | `saf.Worker.deliverRow` | GW | **Fixed** in #127: `saf.Worker` verifies every advice response with `purchase.MACVerifier` (the live ZAK plus the PENDING/retired-key fallback, DE 64 or 128) as its x30 (0430, or 0230 for a completion repeat). A mismatch is logged, counted in `mcn_mac_failure_total`, never acknowledged, and the advice is retried |
| NET-G21 | The issuer's business date isn't cutover-aware: it files a transaction under the JVM's `LocalDate.now()`, while the gateway (ADR-007, #120) rolls its business date at `CUTOVER_TIME` (23:59:59 local). A request sent near cutover can be filed under different business dates on the two sides, so their per-day totals disagree | Issuer: `LocalDate.now()` for the business date; gateway: `bizdate.ClockCalendar` | ISS | Deferred to MCN-702 (cutover 0800/201 and 0500 reconciliation, Sprint 9+): the issuer takes the business date from the acquirer's cutover. Not fixed now |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version, verified against main @ `8d27c72` and the local stack (GET only) |
| 1.1 | 2026-09-25 | NET-G1–G4, G7, G9, G12–G16 fixed on the provider side (#121) |
| 1.2 | 2026-09-26 | NET-G18 added; `ECHO_TIMEOUT` is validated (≤ 15 s) so manual triggers stay under the HTTP WriteTimeout (#121 review) |
| 1.3 | 2026-09-26 | NET-G8, NET-G10, NET-G11 fixed on the web side (#124) |
| 1.4 | 2026-09-26 | NET-G15 web half fixed (#125) |
| 1.5 | 2026-09-26 | NET-G19 (issuer 0430 unsigned) Fixed in #122; NET-G20 (gateway doesn't verify the 0430 MAC) added, open. |
| 1.6 | 2026-09-26 | NET-G5 fixed (#117) |
| 1.7 | 2026-09-26 | NET-G21 added (issuer business date not cutover-aware), deferred to MCN-702 |
| 1.8 | 2026-09-26 | NET-G20 fixed (#127): the SAF worker verifies the 0430 MAC |
