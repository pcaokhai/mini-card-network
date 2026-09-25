# MCN-304 follow-up: a journey the canvas can render

Branch `feat/MCN-304-journey-canvas`, lane GW (`gateway-go/` only). Builds on contract PR #92 (`JourneyStep.code`, `StepCode`).

**Goal.** `GET /v1/transactions/{rrn}/journey` returns the step sequence of the design canvas's "Hành trình giao dịch" screen: every step carries `code`, `actor`, a monotonic `offsetMs`, `kind`, English fallback copy, and, on the network steps, an `IsoMessage` rebuilt from stored columns. `latencyMs` is filled on list, detail and journey (gap G6 in `docs/api/overview-page.md`).

## Interfaces

- `journey.BuildJourney(txn store.TranLogRow, history []store.StateTransition, rev *journey.Reversal) journey.Journey`
- `journey.Reversal{Status string; Attempts int; QueuedAt time.Time; AckedAt *time.Time; Fields map[int]string}`: the stored 0420 (no DE 2) plus its saf_queue bookkeeping.
- `journey.Step` gains `Code StepCode` and `Message *IsoMessage`; `journey.IsoMessage{MTI, Fields []IsoField}`, `journey.IsoField{DE, EasyName, TechnicalName, Format, Value}` (JSON-tagged per the contract).
- `journey.LatencyMs(txn store.TranLogRow) *int`: `responded_at - sent_at` when the issuer answered.
- `store.TranLogRow.RespondedAt *time.Time` (selected, not newly written: `UpdateStatus` already sets it).
- `store.SafRow` gains `CreatedAt`, `AckedAt`; `(*store.SafRepository).FindReversal(ctx, tranID) (SafRow, error)`: the latest 0420 row, `ErrNotFound` when none.
- `saf.NewReversalLookup(repo, encKey)`, `(*saf.ReversalLookup).Reversal(ctx, tranID) (*journey.Reversal, error)`: decrypts the stored advice; nil when none.
- `api.MountTransactionsQuery(r, reader, reversals ReversalReader)`; the lookup is read only when the history has a `REVERSAL_PENDING` transition.

## Sequences

| Outcome | Steps |
| --- | --- |
| approved | POS_REQUEST, REQUEST_SENT (0200), ISSUER_APPROVED (0210), POS_RESULT |
| declined | POS_REQUEST, REQUEST_SENT, ISSUER_DECLINED (0210, BAD), POS_RESULT (BAD) |
| link-down decline | POS_REQUEST, LOCAL_DECLINE, POS_RESULT (BAD) |
| still in flight (SENT) | POS_REQUEST, REQUEST_SENT |
| timeout | POS_REQUEST, REQUEST_SENT, NO_RESPONSE, REVERSAL_QUEUED, POS_RESULT (BAD), REVERSAL_SENT (042x), REVERSAL_CONFIRMED (0430) |
| cancellation / MAC failure | response steps, POS_RESULT, REVERSAL_QUEUED, REVERSAL_SENT, REVERSAL_CONFIRMED |
| any | LATE_RESPONSE inserted at its own time |

A pending reversal stops after REVERSAL_QUEUED (no STAN assigned, no attempt) or REVERSAL_SENT.

## Tests (all end in `__MCN_304`)

journey: `TestBuildJourney_sequencesByOutcome` (table: codes, actors and kinds per outcome row above), `TestBuildJourney_offsetsAreMonotonicFromFirstEvent`, `TestBuildJourney_lateResponseLandsAtItsOwnOffset`, `TestBuildJourney_reversalSentIs0421AfterFailedAttempts`, `TestBuildJourney_messagesCarryCanvasFieldSets` (DE list per MTI), `TestBuildJourney_de2IsMaskedPanOnly` (masking test: first 6 + last 4, never a clear PAN even if one leaked into the row), `TestBuildJourney_moneyDeltas` (debit at ISSUER_APPROVED / NO_RESPONSE, credit at REVERSAL_CONFIRMED, none for a decline), `TestLatencyMs`.
store: `TestSafRepository_findReversalReturnsLatest0420`, `TestTranLogRepository_getReadsRespondedAt`. saf: `TestReversalLookup_decodesStoredAdvice`. api: `TestGetTransactionJourney_embedsCodesAndMessages`, `TestGetTransactions_fillsLatencyMs`.

## Rulings

1. **DE 64 / DE 128 (MAC) omitted.** The MAC is computed per send and never stored.
2. **DE 52 (PIN block) omitted.** Never forwarded or stored (risk R-12); the canvas's DE 52 row has no source.
3. **DE 2 is the stored masked PAN**, passed through `obs.MaskPAN` again so a clear PAN can never reach the response even if one were stored by mistake.
4. **0420 fields come from the stored advice**, not rebuilt: the SAF payload is the advice the worker sends (DE 3, 4, 7, 11, 37, 39, 41, 42, 49, 90), so DE 90 is exactly the `saf.reversalAdvice` layout and DE 7/11 are the advice's own. Before its first send the advice has no DE 7/11, so they are omitted.
5. **0210 and 0430 are reconstructed** from the request's stored columns plus the stored RC/auth code (echo fields per docs/03 §3). The 0430's RC is "00": `MarkAcked` runs only on a "00" ACK.
6. **Attempts.** `saf_queue.attempts` counts failed sends only, so an ACKed advice took `attempts + 1` sends; REVERSAL_SENT shows 0421 when that is more than one. Its offset is the advice's DE 7 (the first send, second precision, year taken from the queue time).
7. **Money.** Delta semantics kept, `balanceAfter` null. The debit sits at ISSUER_APPROVED; an unknown outcome (timeout) is treated as held, so its debit sits at NO_RESPONSE; a MAC-failure decline that queued a reversal is debited at ISSUER_DECLINED. A plain decline moves no money (previously it showed a debit). The credit sits at REVERSAL_CONFIRMED.
8. **Offsets** are measured from the earliest of `created_at` / `sent_at` and clamped to be non-decreasing, since `tran_state_history` and `saf_queue` timestamps come from separate statements.
9. **latencyMs** is null when there was no issuer response (timeout, link-down, still in flight).

## AC

| AC | Evidence |
| --- | --- |
| Steps carry code/actor/offset/kind per outcome | `TestBuildJourney_sequencesByOutcome__MCN_304` |
| ISO messages on network steps, masked DE 2 | `TestBuildJourney_messagesCarryCanvasFieldSets__MCN_304`, `TestBuildJourney_de2IsMaskedPanOnly__MCN_304` |
| Reversal delivery detail | `TestBuildJourney_reversalSentIs0421AfterFailedAttempts__MCN_304`, `TestReversalLookup_decodesStoredAdvice__MCN_304` |
| latencyMs filled (G6) | `TestLatencyMs__MCN_304`, `TestGetTransactions_fillsLatencyMs__MCN_304` |
