# MCN-401 fix — 0420 reversal advices that the issuer can actually process

> **For Claude:** executing-plans (tightly coupled GW + ISS), TDD per step. Security-sensitive (PAN, MAC): code-reviewer pass before merge (root CLAUDE.md §4.7).

**Goal:** a cancellation, a purchase timeout or a response-MAC failure ends with the issuer reversing the money and the acquirer's `tran_log` reading `REVERSED`, on the real stack.

**Found by:** running the MCN-002 acquirer seed against a fresh stack (2026-09-25); the user chose to fix it before seeding reversals. Root CLAUDE.md §6.4 ("unknown outcome ⇒ reversal") had never worked end to end.

**Spec:** `contracts/iso8583/vectors/0420-reversal-timeout.json` (fields 2, 3, 4, 7, 11, 12, 13, 15, 22, 25, 32, 37, 39, 41, 42, 49, 90, 128), `docs/03-iso8583-interface-spec.md` §3 table (0420 mandatory fields), §5 (MUX key DE 11 + DE 7), §7.3 (DE 90 layout: acquirer ID right-justified zero-filled), §11 (MAC).

## What was wrong (evidence)

| # | Defect | Evidence |
| --- | --- | --- |
| 1 | The queued 0420 carried only DE 4, 37, 39, 41, 42, 49, 90: no DE 2, 3, 7, 11, 12, 13, 15, 22, 25, 32 or MAC | `saf.reversalFields` vs the golden vector |
| 2 | A cancellation's 0420 had a DE 90 with a blank STAN: `tran_log` reads never selected `network_stan`, and `%6s` turned "" into spaces | SAF `lastError`: `pack request: INVALID_VALUE: DE 90: must be numeric` |
| 3 | DE 90's original DE 7 came from `tran_log.created_at`, not the DE 7 actually sent | `reversalFields` |
| 4 | Without DE 11/7 the MUX key was empty, so no 0430 could ever be matched | `isonet.Mux.Send` keys on (DE 11, DE 7) |
| 5 | Send failures were swallowed: attempts rose with `lastError` null | `saf.Worker.deliverRow` |
| 6 | The issuer only trimmed spaces from DE 90's acquirer ID, so a spec-compliant zero-filled `00000970499` never matched `970499` | `ParseReversal`, its test used a space-padded value |

## Rulings

1. **No PAN at rest.** `tran_log` gains `card_token` (the simulator's non-sensitive handle, migration 00006). The SAF payload stores the token. The worker resolves the PAN from the card-token registry only while building the frame, the same way `purchase.Service` builds a 0200's DE 2.
2. **STAN and DE 7 are assigned on the first delivery attempt**, not when queued, because a reversal is often queued while the link is down and there is no STAN to allocate. They are written back into the payload before the send, so every 0421 repeat is the same message with only the MTI changed (docs/03 §4).
3. **The MAC (DE 128, since DE 90 sets the secondary bitmap) is computed per send**, because the MTI (0420 → 0421) is part of the MACed message.
4. **Original data** comes from what the purchase actually sent: `tran_log.sent_at` holds the DE 7 moment, and DE 12/13/15 and DE 90 are derived from it. `processing_code` and `pos_entry_mode` are stored at insert.
5. The 0430 is still not MAC-verified by the gateway, and the issuer does not sign it; docs/03 §7.3 advices are never declined. Out of scope, noted.

## Tests (failing first)

- GW store: `TestTranLogRepository_roundTripsTheFieldsAReversalNeeds__MCN_401`.
- GW purchase: `TestCreatePurchase_recordsWhatA0420MustRepeat__MCN_401` (token, DE 3, DE 22, DE 7 moment).
- GW saf: `TestReversalAdvice_matchesTheGoldenVectorFieldSet__MCN_401`, `TestReversalAdvice_de90IsNumericAndZeroFilled__MCN_401`, `TestWorker_assignsStanOnceAndRepeatsTheSameMessage__MCN_401`, `TestWorker_macsEverySend__MCN_401`, `TestWorker_neverPersistsThePAN__MCN_401`, `TestWorker_recordsWhyADeliveryFailed__MCN_401`.
- ISS: `prepare_stripsTheZeroFillFromTheAcquirerId__MCN_401`.
- Manual: seed with cancellations on a fresh stack → both reach `REVERSED`; the issuer ledger shows the reversing journal.
