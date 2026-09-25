# ISO 8583 Interface Specification (MCN-87A)

Version 1.0 · 2026-09-21 · Owner: Tech Lead · Applies to: gateway-go, switch, issuer-jpos

This is the wire contract between the acquirer gateway (and later the switch) and the issuer. It is based on ISO 8583:1987 with an ASCII encoding we call **MCN-87A**. Where this document and a library default disagree, this document wins. Machine-readable parts live in `contracts/iso8583/` (`packager-spec.yaml`, `vectors/*.json`).

## 1. Transport

| Item | Rule |
| --- | --- |
| Protocol | TCP, persistent, full duplex; the acquirer side (gateway/switch) is the client |
| Framing | 2-byte unsigned big-endian length header, value = number of message bytes that follow (header excluded). jPOS `NACChannel` without TPDU; Go `isonet.Framer` |
| Max message size | 4096 bytes; larger frames are rejected and the connection is closed |
| Multiplexing | Many requests in flight on one connection; responses can arrive in any order |
| Connections | One logical link per counterparty; up to 2 physical connections (pool) from Sprint 10 |
| TLS | Not used in the lab (loopback). Documented as required for production |

## 2. Encoding

| Element | Encoding |
| --- | --- |
| MTI | 4 ASCII digits |
| Bitmap | ASCII hex, 16 characters per 64-bit bitmap, uppercase. Bit 1 set ⇒ secondary bitmap (16 more hex chars) follows |
| Numeric `n` fixed | ASCII digits, left-padded with `0` |
| `an` / `ans` fixed | ASCII, right-padded with spaces |
| LLVAR / LLLVAR | 2 or 3 ASCII digit length prefix, then data |
| Binary `b` | ASCII hex (2 chars per byte); length prefixes count **bytes**, not hex chars |
| Track 2 `z` | Not used in v1 (chip and manual only) |

## 3. Data elements used

| DE | Name | Format | Notes |
| --- | --- | --- | --- |
| 1 | Secondary bitmap | b 64 | Present when any DE 65–128 is present |
| 2 | PAN | n..19 LLVAR | Masked in every log |
| 3 | Processing code | n 6 | §6 |
| 4 | Amount, transaction | n 12 | Minor units |
| 7 | Transmission date/time | n 10 MMDDhhmmss | UTC; set by the message originator |
| 11 | STAN | n 6 | §5 |
| 12 | Time, local transaction | n 6 hhmmss | Terminal local time (Asia/Ho_Chi_Minh) |
| 13 | Date, local transaction | n 4 MMDD | |
| 14 | Expiration date | n 4 YYMM | |
| 15 | Settlement (business) date | n 4 MMDD | Set by the acquirer at send time |
| 18 | Merchant category code | n 4 | |
| 22 | POS entry mode | n 3 | `051` chip+PIN, `052` chip no PIN, `011` manual+PIN, `012` manual no PIN |
| 25 | POS condition code | n 2 | `00` normal; `06` pre-auth |
| 32 | Acquiring institution ID | n..11 LLVAR | Lab acquirer `970499` |
| 37 | RRN | an 12 | §5 |
| 38 | Authorization ID response | an 6 | Issuer-assigned on approval |
| 39 | Response code | an 2 | §8 |
| 41 | Terminal ID | ans 8 | |
| 42 | Merchant ID | ans 15 | |
| 43 | Card acceptor name/location | ans 40 | 25 name + 13 city + 2 country |
| 48 | Additional data, private | ans..999 LLLVAR | Key change payload only (§11): `KT=ZPK;KC=<cryptogram under ZMK, hex>;KCV=<6 hex>` |
| 49 | Currency code, transaction | n 3 | `704` |
| 52 | PIN block | b 8 | ISO 9564 format 0 under ZPK |
| 53 | Security related control info | n 16 | Key change: key type + key index |
| 54 | Additional amounts | an..120 LLLVAR | Balance inquiry response |
| 55 | ICC data | b..255 LLLVAR | EMV TLV (§11) |
| 64 | MAC | b 8 | When no DE 65–128 present |
| 70 | Network management code | n 3 | §7.1 |
| 74–77, 86–89, 97 | Reconciliation counts/amounts, net | n 10 / n 16 / x+n 16 | 0500/0510 only |
| 90 | Original data elements | n 42 | §7.3 |
| 128 | MAC | b 8 | When any DE 65–127 present |

## 4. Message catalogue and field presence

M = mandatory, C = conditional (rule in note), O = optional, E = echo from request, – = absent.

| DE | 0100 | 0110 | 0200 | 0210 | 0220 | 0230 | 0420/0421 | 0430 | 0800 | 0810 | 0500 | 0510 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2 | M | E | M | E | M | E | M | E | – | – | – | – |
| 3 | M | E | M | E | M | E | M | E | – | – | – | – |
| 4 | M | E | M | C1 | M | E | M | E | – | – | – | – |
| 7 | M | E | M | E | M | E | M | E | M | E | M | E |
| 11 | M | E | M | E | M | E | M | E | M | E | M | E |
| 12, 13 | M | E | M | E | M | E | M | E | – | – | – | – |
| 14 | M | – | M | – | O | – | O | – | – | – | – | – |
| 15 | M | E | M | E | M | E | M | E | C4 | C4 | M | E |
| 18 | M | – | M | – | M | – | O | – | – | – | – | – |
| 22 | M | – | M | – | M | – | M | – | – | – | – | – |
| 25 | M | – | M | – | M | – | M | – | – | – | – | – |
| 32 | M | E | M | E | M | E | M | E | O | E | M | E |
| 37 | M | E | M | E | M | E | M | E | – | – | – | – |
| 38 | – | C2 | – | C2 | M | – | O | – | – | – | – | – |
| 39 | – | M | – | M | – | M | M (reason) | M | – | M | – | M |
| 41, 42 | M | E | M | E | M | E | M | E | – | – | – | – |
| 43 | M | – | M | – | O | – | – | – | – | – | – | – |
| 49 | M | E | M | E | M | E | M | E | – | – | M | E |
| 52 | C3 | – | C3 | – | – | – | – | – | – | – | – | – |
| 48, 53 | – | – | – | – | – | – | – | – | C5 | C5 | – | – |
| 54 | – | – | – | C6 | – | – | – | – | – | – | – | – |
| 55 | C7 | C7 | C7 | C7 | O | – | O | – | – | – | – | – |
| 64/128 | M | M | M | M | M | M | M | M | – | – | M | M |
| 70 | – | – | – | – | – | – | – | – | M | E | – | – |
| 74–77, 86–89, 97 | – | – | – | – | – | – | – | – | – | – | M | E |
| 90 | – | – | – | – | C8 | – | M | E | – | – | – | – |

Notes: **C1** partial approval returns the approved amount in DE 4 with RC `10`. **C2** present when RC = `00`/`10`. **C3** present when DE 22 position 3 = `1`. **C4** present with DE 70 = `201` (new business date). **C5** present with DE 70 = `161`. **C6** balance inquiry (DE 3 `31xxxx`). **C7** present when DE 22 starts with `05`. **C8** completion references the pre-auth.

Processing is by MTI class: `01` authorization, `02` financial, `04` reversal, `05` reconciliation, `08` network management. Function digit 0 request, 1 response, 2 advice, 3 advice response; x21 = repeat of x20 (same content, only MTI changes).

## 5. Identifiers

| Identifier | Rule |
| --- | --- |
| STAN (DE 11) | Assigned by the message originator per link, sequence `000001`–`999999`, cycles. Unique among in-flight messages on the link |
| MUX key | (DE 11, DE 7) on the gateway; (DE 32, DE 41, DE 11, DE 7) at the issuer |
| RRN (DE 37) | 12 chars: `Y` last digit of year + `DDD` day of year + `hh` UTC hour + STAN. Unique per acquirer per day; carried unchanged into reversals and advices |
| Dedupe key (issuer) | (DE 32, DE 41, DE 11, DE 7 raw, MTI class, business date). Repeat x21 is normalized to x20 before dedupe |
| Terminal STAN vs network STAN | The POS has its own STAN towards the gateway; the gateway assigns a new network STAN. Both are stored |

## 6. Processing codes (DE 3)

| Code | Meaning |
| --- | --- |
| `000000` | Purchase |
| `010000` | Cash withdrawal (not in POS v1; used in tests) |
| `200000` | Refund |
| `310000` | Balance inquiry |
| `000000` + DE 25 `06` | Pre-authorization (with MTI 0100) |

## 7. Message flows and rules

### 7.1 Network management (DE 70)

| DE 70 | Meaning | Who sends |
| --- | --- | --- |
| 001 | Sign-on | Acquirer on connect |
| 002 | Sign-off | Acquirer before disconnect |
| 161 | Key change (new ZPK/ZAK in DE 48 under ZMK, key type as a `ZPK:`/`ZAK:` prefix in DE 48) | Issuer or acquirer |
| 201 | Cutover (DE 15 = new business date) | Acquirer |
| 301 | Echo test | Either side |

Financial messages MUST NOT be sent on a link that is not signed on. The issuer answers any request on a signed-off link with RC `91`.

### 7.2 Authorization / financial

Request 0100/0200 → response 0110/0210 within the timeout (§9). The issuer MUST respond to every request, including on internal failure (RC `96`). Partial approval (RC `10`) only when DE 3 is purchase and the card allows it (Sprint 8).

### 7.3 Reversal (0420/0421/0430)

- Sent by the acquirer when the outcome of an original request is unknown (timeout, disconnect after send) or when the POS cancels.
- DE 39 in 0420 carries the reason: `68` response received too late / timeout, `17` customer cancellation, `06` error.
- DE 90 layout (n 42): original MTI (4) + original STAN (6) + original DE 7 (10) + original acquirer ID right-justified zero-filled (11) + forwarding institution ID (11, zeros in v1).
- The issuer MUST acknowledge with 0430 once the reversal is durably recorded, whether or not the original was found. Advices cannot be declined.
- If the original is not found, the issuer records `REVERSAL_WITHOUT_ORIGINAL`; if the original arrives later it is declined with RC `94` and not posted.
- A reversal of an already reversed transaction is acknowledged idempotently with no ledger effect.

### 7.4 Advices (0120/0220)

Completion (0220) and STIP advices (0120/0220 from the switch) are advices: store-and-forward, repeat as x21 until the x30 response, never declined; the issuer answers 0x30 with RC `00` after durable recording (business rejection is handled in reconciliation).

### 7.5 Duplicates

The issuer MUST treat a request whose dedupe key already exists as a duplicate and replay the stored response (same DE 38, 39, 4). It MUST NOT post again. Metrics: `mcn_duplicate_total`.

### 7.6 Cutover and business date

The acquirer sends 0800/201 with the new DE 15 at the configured cutover time (default 23:59:59 local). Messages carry the DE 15 assigned when they were sent; a message sent before cutover belongs to the old business date even if processed after. After cutover the acquirer sends 0500 for the closed date.

### 7.7 Reconciliation (0500/0510)

0500 carries acquirer totals for the closed business date: DE 74 credits count, 75 credit reversals count, 76 debits count, 77 debit reversals count, 86–89 corresponding amounts (n 16), 97 net amount (`C`/`D` + n 16). The issuer compares with its own totals and answers 0510 with RC `00` in balance or `95` out of balance (detail resolved in settlement).

## 8. Response codes (DE 39)

| RC | Meaning | Issued when |
| --- | --- | --- |
| 00 | Approved | |
| 05 | Do not honor | Generic decline, velocity rule (v1 fallback) |
| 06 | Error | Reversal reason only |
| 10 | Partial approval | Sprint 8 |
| 12 | Invalid transaction | Unsupported processing code / MTI |
| 13 | Invalid amount | Amount ≤ 0 or above system max |
| 14 | Invalid card number | Unknown PAN |
| 17 | Customer cancellation | Reversal reason only |
| 30 | Format error | Packager/validation failure (issuer responds with the fields it could parse) |
| 51 | Insufficient funds | |
| 54 | Expired card | |
| 55 | Incorrect PIN | |
| 57 | Transaction not permitted to cardholder | Card type/entry mode not allowed |
| 61 | Exceeds amount limit | Daily or per-transaction limit |
| 62 | Restricted card | Card status BLOCKED/LOST/STOLEN (v1 uses 62 for all; 41/43 reserved) |
| 65 | Exceeds frequency limit | Velocity count |
| 68 | Response received too late | Reversal reason; gateway → POS on timeout |
| 75 | PIN tries exceeded | |
| 91 | Issuer or switch inoperative | Link down / not signed on |
| 94 | Duplicate transmission | Original arriving after its reversal |
| 95 | Reconciliation error | 0510 out of balance |
| 96 | System malfunction | Unexpected error |

## 9. Timers

| Timer | Value | Owner |
| --- | --- | --- |
| Request timeout 0100/0200 | 30 s | Acquirer MUX |
| Advice ack timeout | 10 s | SAF worker |
| Echo interval | 60 s | Acquirer and switch |
| Link down after | 3 consecutive echo failures | Acquirer and switch |
| Issuer processing target | p99 < 100 ms | Issuer |
| Reversal grace for new key (DE 70 = 161) | 5 min old key accepted | Both |

## 10. Correlation and tracing

ISO messages carry no trace headers. Each host logs `trace_id` with (DE 11, DE 7, DE 37) when it sends or receives a message, and stores `trace_id` in `tran_log`. Cross-host traces are joined by RRN in Tempo/Grafana via span attributes `iso.rrn`, `iso.stan`, `iso.mti`.

## 11. Security

- **PIN block:** ISO 9564-1 format 0: `0` + PIN length (hex) + PIN + `F` padding, XOR `0000` + 12 rightmost PAN digits excluding the check digit. Encrypted under TPK at the POS, translated to ZPK by the gateway, verified via PVV by the issuer.
- **MAC:** ISO 9797-1 algorithm 3 (Retail MAC, "X9.19") over the full packed message excluding the MAC field, using ZAK; last 8 bytes placed in DE 64 (or DE 128 when secondary bitmap present). A MAC failure is answered with RC `96` and counted in `mcn_mac_failure_total`.
- **EMV (DE 55):** TLV; v1 recognizes tags 9F26 (ARQC), 9F27 (CID), 9F10 (IAD), 9F36 (ATC), 9F37 (unpredictable number), 95 (TVR), 9A (date), 9C (type), 5F2A (currency). ARQC verification is simulated with a keyed HMAC in the lab; ARPC returned in tag 91.
- **Key change:** new double-length key as a cryptogram under ZMK in DE 48, as `<KEYTYPE>:<cryptogram hex>` (`ZPK:` or `ZAK:`). DE 53 is not sent (MCN-504 ruling, PRs #60/#61: the key type travels in DE 48 so both packagers need no extra field). The receiver activates the key after 0810 RC `00` and keeps the previous key for 5 minutes.

## 12. Golden vectors

`contracts/iso8583/vectors/` holds JSON test vectors (fields + expected packed hex) for every MTI in §4, including edge cases: secondary bitmap, LLLVAR at max length, empty optional fields, invalid length prefixes. Both codecs MUST pass all vectors in CI (`make contracts`). Adding a field or message type starts with a vector.

## 13. Change control

Any change to this spec is a PR to `docs/03` + `contracts/iso8583/` reviewed by ISS and GW owners; breaking changes need an ADR and a version bump (MCN-87A v2).
