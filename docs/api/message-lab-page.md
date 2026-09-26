# Message Lab page: API contract

| | |
| --- | --- |
| Document | `docs/api/message-lab-page.md` |
| Version | 1.0 |
| Status | Approved for integration |
| Date | 2026-09-25 |
| Screen | route `/lab/message`, container `web-next/src/app/(console)/lab/message/MessageLabScreen.tsx` |
| Stories | MCN-104 (WEB), MCN-103 (GW) |
| Provider(s) | gateway-go (`internal/api/lab.go`, `internal/lab/`, codec `internal/iso8583/`) |
| Consumer | web-next BFF `src/app/api/[...path]/route.ts`, then `src/shared/api/lab-client.ts` |
| Design reference | canvas `project/MessageLab.dc.html` (docs/01-prd.md §7.1); plan `docs/plans/MCN-104-lab-canvas.md`; release notes `docs/releases/R5.3.md` "Phòng lab message" |
| Verified against | main @ `8d27c72` plus the running local stack on 2026-09-25 (GET only, plus `POST …/decode`, which is a pure function with no side effects) |

Normative sources, in precedence order: accepted ADRs, `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/03-iso8583-interface-spec.md`, then this document. This document adds what a schema can't express:
- which UI element reads each field, in Easy and Expert modes;
- when each call is made (trigger and cadence);
- ordering, bucketing and derivation rules;
- idempotency and concurrency;
- the error and empty behaviour.

Shared conventions are in [README](README.md) §3 and are not repeated.

## 1. Purpose and scope

The Message Lab lets a learner dissect an ISO 8583 (MCN-87A ASCII) message. The raw wire string is split into coloured segments (MTI, bitmap, one per data element), with an 8×8 bitmap grid, a detail card and a field table that share one selection (MCN-104-AC1…AC5). This contract covers the two calls the page makes (`GET /v1/lab/messages/samples`, `POST /v1/lab/messages/decode`) and documents `POST /v1/lab/messages/encode`, which the provider serves but the page does not call.

Out of scope: a free-text paste box (not in the canvas, Ruling R1 of the plan), and the ISO codec itself (`docs/03-iso8583-interface-spec.md`, golden vectors in `contracts/iso8583/vectors`).

## 2. Page map

| UI region (canvas / i18n label) | Data shown | Call(s) (§4.x) | Refresh |
| --- | --- | --- | --- |
| Title "Phòng lab message" + subtitle (`lab.title`, `lab.subtitle`) | static copy | none | none |
| Sample tabs "0200 Mua hàng", "0210 Trả lời", "0420 Hủy", "0800 Kiểm tra kết nối" (`lab.samples.{mti}`; group label `lab.samplesLabel` "Chọn message mẫu") | one tab per sample, in provider order; label falls back to the provider's `label` when no `lab.samples.{mti}` key exists | §4.1 | once per page load (TanStack Query cache) |
| "Message thô, đúng như gửi trên đường truyền" (`lab.raw.heading`) | coloured segments: `mti`, bitmap (primary + secondary joined), then one per field | §4.2 `segments[]`, `primaryBitmap`, `secondaryBitmap` | on tab change |
| Raw note | Easy `lab.raw.noteEasy` "Mỗi ô màu là một phần của message. Số thẻ đã được che để bảo mật."; Expert `lab.raw.noteExpert` "… · tổng {count} field" | §4.2 `fields.length` | on tab change |
| "Lưới bitmap" with pages "Chính 1–64" / "Phụ 65–128" (`lab.bitmap.*`) | 8 rows × 8 bits, per-row hex and binary, total bitmap hex | §4.2 `primaryBitmap`, `secondaryBitmap` | on tab change |
| Dark detail card ("Chi tiết", `lab.detail.*`) | title, why, technical name, format, value, status, parts (MTI digits, DE 22, DE 55 tags, DE 90 parts) | §4.2 `mti`, `fields[]` + client glossary (`lab.fields`, `lab.mti`, `lab.entry`, `lab.emv`, `lab.original`) | on selection |
| "Danh sách field" table (`lab.list.*`) | Easy: Field · "Ý nghĩa" · "Giá trị"; Expert: Field · "Tên kỹ thuật" · "Định dạng" · "Giá trị" | §4.2 `fields[]` | on tab change |
| Error line | `lab.loadFailed` "Không tải được message: {detail}" | §4.1 or §4.2 problem `detail` | on failure |
| Loading line | `lab.loading` "Đang tải message mẫu…" | §4.1 / §4.2 pending | while pending |

## 3. Call inventory

| # | Method + path | Provider | Purpose | Trigger / cadence | Idempotency-Key | Concurrency (If-Match/ETag) | Availability |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.1 | `GET /v1/lab/messages/samples` | gateway-go | The four golden sample messages (the tabs) | once on mount; TanStack defaults (stale immediately, refetch on window focus) | n/a (GET) | none | Real |
| 4.2 | `POST /v1/lab/messages/decode` | gateway-go | Break one raw message into MTI, bitmaps, segments and fields | on mount for sample 0, then on every tab change; cached per `raw` forever (`staleTime: Infinity`) | not sent, not required (read-only POST) | none | Real |
| 4.3 | `POST /v1/lab/messages/encode` | gateway-go | Pack fields into a message and return the same breakdown | never called by this page | not sent, not required | none | Real (no consumer) |
| – | WebSocket | – | – | – | – | – | None consumed |
| – | Client-only data | web-next | Field names, explanations and formats for 27 DEs (`FIELD_SPECS` in `src/components/lab/lab-model.ts`, `lab.fields` in `messages/vi.json`); MTI digit, DE 22, EMV tag and DE 90 glossaries | static | – | – | Real |

## 4. Calls

### 4.1 GET /v1/lab/messages/samples

- **Summary.** Returns the four golden vectors as labelled samples: 0200, 0210, 0420, 0800, in that order.

**Request.** No path or query parameters, no body.

| Header | Required | Notes |
| --- | --- | --- |
| `traceparent` | no | forwarded by the BFF; the provider does not echo a trace id |

**Response.** `200 OK`, inline array schema `{ mti, label, raw }` (contracts/openapi.yaml `listSampleMessages`).

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `[].mti` | string, 4 digits | ✓ | tab label key `lab.samples.{mti}` | "0200 Mua hàng" etc. | same |
| `[].label` | string (English) | ✓ | tab label fallback when `lab.samples.{mti}` is missing | shown only as fallback | same |
| `[].raw` | string, ASCII packed message without length header | ✓ | input to §4.2; tab React key | not shown directly | not shown directly |

**Provider rules.**
1. The list is a compiled-in constant (`gateway-go/internal/lab/samples.go`), not read from `contracts/iso8583/vectors` at runtime. It always has exactly four entries, in the order 0200, 0210, 0420, 0800.
2. `raw` is the golden vector byte for byte. It carries the test PAN in clear (BIN 970436 test card), the DE 52 PIN block and the DE 64 MAC (see §6 and LAB-G1).
3. Two tabs are labelled by MTI only, so two samples with the same MTI would share a label. The provider never sends duplicate MTIs today.

**Errors.** The handler has no error path: it always answers 200. The BFF adds one.

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | the gateway is unreachable | red line "Không tải được message: {detail}"; no tabs; no dissection |

**Example.** Real, local stack, 2026-09-25, `raw` of the first three trimmed.

```json
[
  { "mti": "0200", "label": "Purchase, chip + PIN", "raw": "0200723E448108E0920116970436…4E1D7B02C9A3F815" },
  { "mti": "0210", "label": "Purchase approved", "raw": "0210723A00010EC08001169704…91C0A4E27D3B5F68" },
  { "mti": "0420", "label": "Reversal on timeout", "raw": "0420F23A04810AC080000000004…5D0E33A17BC2904F" },
  { "mti": "0800", "label": "Echo (network management)", "raw": "0800822000000000000004000000000000000921073300000200301" }
]
```

**Notes.** No `ETag` or `Cache-Control`; the client caches under query key `["lab","samples"]` with TanStack defaults (3 retries with exponential backoff, refetch on window focus). Rate limit: not enforced.

### 4.2 POST /v1/lab/messages/decode

- **Summary.** Unpacks one raw message and returns the MTI, bitmaps, ordered raw segments and one entry per present data element, with names and formats.

**Request.**

| Header | Required | Notes |
| --- | --- | --- |
| `Content-Type` | yes | `application/json` |
| `Idempotency-Key` | no | not sent by the client and not checked: decode changes no state |
| `traceparent` | no | forwarded by the BFF |

| Body field | Type | Required | Constraints | Notes |
| --- | --- | --- | --- | --- |
| `raw` | string | ✓ | contract `maxLength: 8192`; MCN-87A ASCII, no 2-byte length header | the page always sends a sample's `raw` from §4.1 |

**Response.** `200 OK`, schema `DecodedMessage`.

| Field | Type | Required | UI element | Easy mode | Expert mode |
| --- | --- | --- | --- | --- | --- |
| `mti` | string | ✓ | first raw segment; MTI detail (4 digit parts from the `lab.mti` glossaries) | "Loại message {mti}" | same, plus tech "Message type indicator", format "n 4" |
| `primaryBitmap` | string, 16 hex | ✓ | bitmap segment; grid page "Chính 1–64" | "Toàn bộ bitmap viết dạng hex" | "Bitmap 64 bit (hex)" |
| `secondaryBitmap` | string, 16 hex, or absent | – | grid page "Phụ 65–128" (disabled while absent, plan R4); appended to the bitmap segment | as above | "Bitmap 128 bit (hex)" |
| `segments[]` | array `{ key, text }` | ✓ | raw message segments, in wire order; `mti`, `primaryBitmap` and `secondaryBitmap` keys are merged into two client segments | coloured cells, hint "Field {n}: {name}" | same |
| `segments[].text` (key `2`) | string | ✓ | PAN segment | length prefix kept, PAN masked: `16970436******4417` | same |
| `fields[].de` | string, DE number | ✓ | row "Field"; bitmap cell on/off; selection key | `2`, `3`, … | same |
| `fields[].easyName` | string (English) | ✓ | name fallback for a DE the client glossary lacks | shown only as fallback | not shown |
| `fields[].technicalName` | string (English) | ✓ | tech-name fallback for a DE `FIELD_SPECS` lacks | not shown | shown only as fallback |
| `fields[].format` | string (`n`, `an`, `ans`, `b`, …) | ✓ | format fallback for a DE `FIELD_SPECS` lacks | not shown | shown only as fallback (the client's `FIELD_SPECS` format, e.g. `n..19 LLVAR`, wins) |
| `fields[].value` | string | ✓ | row "Giá trị"; detail value; DE 22 / 55 / 90 parts | value (PAN masked `970436******4417`) | same |
| `fields[].raw` | string | – | not read by the UI | not shown | not shown |

**Provider rules.**
1. `segments[]` is in wire order: `mti`, `primaryBitmap`, `secondaryBitmap` when bit 1 is set, then one segment per present DE in ascending number. Concatenating every `text` gives back the request `raw`, except for DE 2, whose value is masked.
2. `fields[]` is sorted by DE number ascending (`sort.Ints` in `internal/lab/decode.go`). DE 1 (the secondary bitmap) is never a field; it is carried in `secondaryBitmap`.
3. `fields[].value` is the value without its LL/LLL length prefix. DE 55 therefore starts at the first EMV tag (`9F26…`), which the client parses as TLV.
4. `easyName` equals `technicalName` except for DE 22 ("Entry mode"), 55 ("Card chip data (EMV)") and 90 ("Reference to the original message") (`internal/lab/glossary.go`).
5. DE 2 is masked to first 6 + last 4 in `fields[].value` and in `segments[]`, with the LL prefix left visible for teaching. `fields[].raw` for DE 2 is **not** masked today (LAB-G1).
6. `secondaryBitmap` is omitted, not `null`, when bit 1 is off.
7. Decoding is deterministic: the same `raw` always gives the same body, which is why the client caches it forever.

**Errors.** Every codec failure is a 400 whose `type` is the codec's error code (`internal/iso8583/codec.go`, `segments.go`) and whose `title` repeats it.

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `invalid-request` | body is not JSON | red line "Không tải được message: {detail}"; the previous dissection stays on screen (`keepPreviousData`) |
| 400 | `TRUNCATED` | `raw` ends before the MTI, bitmap or a field is complete (also for `raw: ""`) | same |
| 400 | `INVALID_MTI` | the first 4 chars are not a valid MTI | same |
| 400 | `INVALID_BITMAP` | a 16-char bitmap is not hex | same |
| 400 | `UNKNOWN_FIELD` | a bit is set for a DE the packager spec doesn't define | same |
| 400 | `INVALID_LENGTH` | an LL/LLL prefix is non-numeric or exceeds the DE's maximum | same |
| 400 | `INVALID_VALUE` | a numeric DE holds non-digits, or a binary DE isn't uppercase hex | same |
| 400 | `TRAILING_DATA` | characters remain after the last field | same |
| 400 | `invalid-message` | any other decode error (fallback in `writeCodecProblem`) | same |
| 502 | `https://mcn.local/problems/upstream-unavailable` (BFF) | gateway unreachable | same |

Since the page only decodes the provider's own samples, these errors occur only if the samples and the codec disagree.

**Example.** Real, local stack, 2026-09-25. Request: the 0800 sample.

```json
{ "raw": "0800822000000000000004000000000000000921073300000200301" }
```

```json
{
  "mti": "0800",
  "primaryBitmap": "8220000000000000",
  "secondaryBitmap": "0400000000000000",
  "segments": [
    { "key": "mti", "text": "0800" },
    { "key": "primaryBitmap", "text": "8220000000000000" },
    { "key": "secondaryBitmap", "text": "0400000000000000" },
    { "key": "7", "text": "0921073300" },
    { "key": "11", "text": "000200" },
    { "key": "70", "text": "301" }
  ],
  "fields": [
    { "de": "7", "easyName": "Transmission date time", "technicalName": "Transmission date time", "format": "n", "value": "0921073300", "raw": "0921073300" },
    { "de": "11", "easyName": "STAN", "technicalName": "STAN", "format": "n", "value": "000200", "raw": "000200" },
    { "de": "70", "easyName": "Network management code", "technicalName": "Network management code", "format": "n", "value": "301", "raw": "301" }
  ]
}
```

Real, the 0200 sample, trimmed to DE 2 and DE 55:

```json
{
  "mti": "0200",
  "primaryBitmap": "723E448108E09201",
  "segments": [ "…", { "key": "2", "text": "16970436******4417" }, "…" ],
  "fields": [
    { "de": "2", "easyName": "PAN", "technicalName": "PAN", "format": "n", "value": "970436******4417", "raw": "…unmasked, see LAB-G1…" },
    "…",
    { "de": "55", "easyName": "Card chip data (EMV)", "technicalName": "ICC data", "format": "b", "value": "9F2608A1B2C3D4E5F607189F2701809F3602001C", "raw": "0209F2608A1B2C3D4E5F607189F2701809F3602001C" }
  ]
}
```

Real error:

```json
{ "type": "INVALID_MTI", "title": "INVALID_MTI", "status": 400, "detail": "INVALID_MTI: ABCD" }
```

**Notes.** Query key `["lab","decode", raw]`, `staleTime: Infinity`, `placeholderData: keepPreviousData`, so switching tabs never blanks the screen and each sample is decoded at most once per page session. Default retries (3). No `ETag`. Rate limit: not enforced.

### 4.3 POST /v1/lab/messages/encode

- **Summary.** Packs `fields` under `mti` and returns the same `DecodedMessage` breakdown as §4.2. **Not called by this page**; documented because it shares the tag and the codec.

**Request.**

| Body field | Type | Required | Constraints | Notes |
| --- | --- | --- | --- | --- |
| `mti` | string | ✓ | `^[0-9]{4}$` | an invalid MTI gives `INVALID_MTI` |
| `fields` | object, DE number (string) → value | ✓ | keys numeric; DE 1 must not be set (derived) | a non-numeric key gives `UNKNOWN_FIELD` |

No `Idempotency-Key` (no state change).

**Response.** `200 OK`, `DecodedMessage` (§4.2), produced by packing and then decoding the result. The packed string itself is **not** returned (LAB-G3).

**Provider rules.** 1. Values are validated per DE: numeric DEs must be digits, binary DEs uppercase hex, fixed-length DEs exact length, variable DEs within their maximum. 2. The bitmap(s) are derived from the keys present.

**Errors.**

| HTTP status | problem `type` | When | UI behaviour |
| --- | --- | --- | --- |
| 400 | `invalid-request` | body is not JSON | n/a (no consumer) |
| 400 | `INVALID_MTI` | `mti` not 4 digits | n/a |
| 400 | `UNKNOWN_FIELD` | non-numeric key, DE 1 set, or a DE not in the packager spec | n/a |
| 400 | `INVALID_VALUE` | numeric DE with non-digits, or binary DE not uppercase hex | n/a |
| 400 | `INVALID_LENGTH` | fixed-length mismatch or variable length over maximum | n/a |
| 400 | `invalid-message` | any other codec error | n/a |

**Example.** From the contract shape; not exercised on the live stack.

```json
{ "mti": "0800", "fields": { "7": "0921073300", "11": "000200", "70": "301" } }
```

The response equals the §4.2 0800 example.

**Notes.** No caching. Rate limit: not enforced.

## 5. Real-time events

None. The page consumes no WebSocket event.

## 6. Security and compliance

| Topic | Rule on this page | Status |
| --- | --- | --- |
| PAN display | DE 2 is shown as first 6 + last 4 (`970436******4417`) in the raw segments, the detail card and the field table. The Easy raw note says so: "Số thẻ đã được che để bảo mật." | Met in `segments[]`, `fields[].value` and `fields[].raw` (#112) |
| PAN in responses | No response field may carry a full PAN (MCN-103-AC4, engineering rule 2). | Met (#112) on every output path (`value`, `raw`, `segments[]`, `samples[].raw`, decode and encode): DE 2 is masked by position at any length (first 6 + last 4, or only the last 4 when 10 digits or fewer); in an `an`/`ans` field (e.g. DE 48), every run of 13 or more consecutive digits is masked the same way (first 6 + last 4), including one glued to letters or `_` (`CARD9704…`, `PAN_9704…`) or longer than 19 digits; runs of 12 digits or fewer are shown; DE 55 is redacted whole, because EMV tags 5A and 57 carry the PAN and track 2 equivalent. Test BIN 970436 only, never a real card |
| PAN in requests | The page POSTs a sample's redacted `raw` back to decode. The provider maps a served sample to its golden vector inside `internal/lab`, so no clear PAN crosses the wire in either direction. | Met (#112) |
| Expiry | DE 14 (expiration date) is shown in clear. PCI DSS allows this once the PAN is masked. | Accepted |
| PIN block, ICC data, MAC | DE 52 (PIN block), DE 55 (ICC data) and DE 64/128 (MAC) are redacted to asterisks of the same length in `value`, `raw`, `segments[]` and `samples[].raw`, on decode and encode. | Met (#112) |
| Key material | None. | – |
| Audit trail | None: every call is read-only. | – |
| Destructive actions | None. | – |

## 7. Non-functional requirements

| Call | Latency budget | Observed on the local stack (2026-09-25, 50 requests, direct to :8080) | Payload | Pagination |
| --- | --- | --- | --- | --- |
| §4.1 samples | no PRD NFR; UI target < 300 ms | p50 1.0 ms, p99 3.3 ms (observed) | 907 B | none; fixed 4 items |
| §4.2 decode | no PRD NFR; UI target < 300 ms | p50 1.0 ms, p99 2.4 ms for the 0200 sample (observed) | request ≤ 8 KiB by contract; response 3.6 KiB for the 0200 sample | none |
| §4.3 encode | none | not measured | – | none |

Load: at most 1 samples call and 4 decode calls per page session (decodes are cached forever). No polling.

## 8. UI states

| State | What renders | Driving call |
| --- | --- | --- |
| Loading | title, subtitle, no tabs yet, "Đang tải message mẫu…" | §4.1 pending, then §4.2 pending |
| Empty | samples returned `[]`: no tabs and the loading line stays, because no decode is ever enabled (LAB-G5) | §4.1 |
| Partial | samples loaded, decode failed: tabs render, red line "Không tải được message: {detail}", and the previous dissection stays if there was one | §4.2 |
| Error | samples failed: red line "Không tải được message: {detail}", no tabs | §4.1 |
| Provider not available | BFF 502: same as Error, `detail` is the fetch error text | §4.1 |
| Success | tabs, raw segments, bitmap grid, detail card (default selection: DE 4, else DE 70, else MTI) and field table | §4.1 + §4.2 |

## 9. Implementation status and gaps

| ID | Gap | Evidence | Owner lane | Proposed fix / story |
| --- | --- | --- | --- | --- |
| LAB-G1 | `fields[].raw` for DE 2 carries the full PAN (with its LL prefix). Only `value` and `segments[]` are masked. Violates MCN-103-AC4 and engineering rule 2. | `gateway-go/internal/lab/decode.go:57` takes `Raw` from `segByDE` (built at `:103` from unmasked segments); live decode of the 0200 sample returned `"raw":"1697…4417"` unmasked | GW | **Fixed** in #112: `fields[].raw` is built from the redacted segments |
| LAB-G2 | `GET /v1/lab/messages/samples` returns `raw` with the test PAN, the DE 52 PIN block and the DE 64 MAC in clear. The decode response also returns DE 52 unmasked. Acceptable only because the vectors are synthetic; the contract (`info.description`: "No PAN/CVV/PIN/track data ever") says otherwise. | `gateway-go/internal/lab/samples.go:14`; live samples response | GW + contracts | **Fixed** in #112: samples are served redacted, same framing; decode maps a served sample to its vector inside `internal/lab`, and DE 52/55/64/128 are redacted everywhere. No ADR or contract change needed |
| LAB-G3 | Encode returns no packed message. MCN-103-AC2 and docs/04 §4 say it "returns the packed message"; `DecodedMessage` has no field for it. | `contracts/openapi.yaml` `DecodedMessage`; `internal/lab/decode.go` `Encode` returns `Decode(packed)` | contracts + GW | **Fixed** in #118: encode returns `packed`, rebuilt from the redacted segments like every Lab output; `null` on decode |
| LAB-G4 | ~~Problem `type` is a bare code (`INVALID_MTI`, `invalid-request`), not the URI form `https://mcn.local/problems/<slug>` of docs/04 §3. There is no `instance` or `traceId`, and codec types are SCREAMING_CASE while the others are kebab-case.~~ | `gateway-go/internal/api/lab.go` `problem()`; live 400 bodies | GW | **Fixed** (#PRN, P-1): URI types, `instance` and `traceId`; codec errors and a malformed body are `validation-error` with the codec code in `errors[]` (`field: raw`) |
| LAB-G5 | An empty samples list leaves "Đang tải message mẫu…" on screen forever. | `MessageLabScreen.tsx`: loading shows while `!decoded && !error`, and decode is disabled without a `raw` | WEB | **Fixed** in #124: an empty samples list shows "Chưa có message mẫu nào để mổ xẻ." |
| LAB-G6 | `maxLength: 8192` on `raw` isn't enforced. The request is bounded only by the server's 5 s `ReadTimeout`. | `internal/api/lab.go` `handleDecode` has no `http.MaxBytesReader` | GW | **Fixed** in #112: `http.MaxBytesReader` on decode and encode; `raw` over 8192 answers 400 `validation-error` |
| LAB-G7 | The provider's `easyName`, `technicalName` and `format` are English and coarse (`n`, `b`). The page ignores them for the 27 DEs in its own `FIELD_SPECS` and glossary, so two sources of truth exist for DE names and formats. | `web-next/src/components/lab/lab-model.ts` `FIELD_SPECS`; plan Ruling R2 | WEB + GW | Accepted for v1 (R2). Long term: return the packager-spec format string (e.g. `n..19 LLVAR`) and drop the client table |
| LAB-G8 | ~~The Lab's redaction set (`secretDEs` in `internal/lab/decode.go`: 52, 55, 64, 128, plus DE 2 by position) duplicates packager-spec's `sensitive` flags (2, 14, 48, 52, 55; none on 64/128), and codegen drops the flag, so the two can drift.~~ | `contracts/iso8583/packager-spec.yaml`; `internal/iso8583/spec_gen.go` has no `Sensitive` field | contracts + GW | **Fixed**: #130 adds `sensitive: mac` to DE 64/128; #PRN generates `Sensitive` into `iso8583.Fields` and the Lab redacts from it (a PIN block, ICC data, a MAC, and any kind added later, are redacted whole; the PAN is masked by position; an expiry and a key-change cryptogram stay visible with free-text PAN masking). `secretDEs` is gone |

## 10. Change log

| Version | Date | Change |
| --- | --- | --- |
| 1.0 | 2026-09-25 | First version, verified against main @ `8d27c72` and the local stack |
| 1.1 | 2026-09-25 | LAB-G1, LAB-G2 and LAB-G6 fixed (#112), including DE 55 redaction and position-based DE 2 masking after security review; §6 updated; LAB-G8 added |
| 1.2 | 2026-09-25 | LAB-G3 fixed (#118) |
| 1.3 | 2026-09-26 | LAB-G5 fixed (#124) |
| 1.4 | 2026-09-26 | LAB-G4 fixed (#PRN, P-1); LAB-G8 fixed (#130 contract, #PRN gateway) |
