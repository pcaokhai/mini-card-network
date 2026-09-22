# Sprint 5 — Execution Overview

> **For Claude:** This file orchestrates Sprint 5. Each story has its own plan (`MCN-40x.md`). Execute each plan with superpowers:subagent-driven-development (or superpowers:executing-plans), one worktree per story, following root `CLAUDE.md` §4. Contract-first check: `contracts/openapi.yaml` (`/v1/transactions/{rrn}/cancellations`, `/v1/network/saf`, `/v1/chaos/scenarios`, `/v1/chaos/runs`) and `contracts/ws-events.schema.json` (`chaos.changed`, `chaos.run.progress`, `saf.changed`) already have every shape this sprint needs from the original doc pack — **no new contract PR is needed before Wave 1 starts**, same situation as Sprint 4.

**Sprint goal:** Money is safe under every failure — a timeout or POS cancellation is durably reversed via SAF, the issuer handles reversals/advices/duplicates without ever corrupting the ledger, a late response can never flip a final transaction, and both the Chaos Lab screen and the failure-scenario journey make this visible and explorable. **Duration:** 2 weeks. **Commitment:** 8 (MCN-401) + 8 (MCN-402) + 3 (MCN-403) + 3 (MCN-404) + 5 (MCN-405) + 3 (MCN-406) = 30 points. **Release:** none this sprint — per `docs/07-delivery-plan.md` §2/§3, R4 is Sprint 6's release (MCN-407's chaos suite + MCN-501's key rotation groundwork complete the slice); Sprint 5 lands on `main` behind `FF_S4A_REVERSAL`/`FF_S4B_CHAOS` same as Sprint 3's shape (no release row).

## Waves

| Wave | Stories (parallel lanes) | Starts when |
| --- | --- | --- |
| 1 | GW **MCN-401** timeout → reversal/SAF ‖ ISS **MCN-402** issuer reversal/advice/duplicate ‖ WEB **MCN-405** Chaos Lab screen ‖ WEB **MCN-406** journey for failure scenarios | Sprint 4 / R3 merged |
| 2 | GW **MCN-403** late response handling → GW **MCN-404** chaos API | Wave 1's MCN-401 merged (both Wave 2 stories depend on MCN-401's `saf_queue`/`tran_log` columns and the `purchase.Service`/`journey` extensions it adds; MCN-404 additionally depends on MCN-401's `saf.Worker` and `journey`'s REVERSAL/WARN vocabulary for a complete chaos-run/journey story, so it is sequenced strictly after MCN-403 within Wave 2 even though its own AC don't require MCN-403's specific columns — matching how Sprint 4 treated the delivery plan's own wave table as authoritative even where a story's dependency list would technically allow more parallelism) |

MCN-405 and MCN-406 (WEB) build against MSW mocks and do not wait for MCN-401/402/403/404, per the delivery plan's "frontend never waits for backend" rule (proven every prior sprint). MCN-406 additionally reuses MCN-307's already-merged `JourneyScreen`/`StepTimeline`/`MoneyPanel` components with additive changes only — confirmed non-overlapping with MCN-405's entirely new `src/components/chaos/` tree.

## Plans

| Plan | Lane | Depends on |
| --- | --- | --- |
| [MCN-401](MCN-401.md) | GW | Sprint 3 / MCN-303 (merged) |
| [MCN-402](MCN-402.md) | ISS | Sprint 3 / MCN-302 (merged) |
| [MCN-405](MCN-405.md) | WEB | Sprint 0 / MCN-003, MCN-004 (merged) |
| [MCN-406](MCN-406.md) | WEB | Sprint 4 / MCN-307 (merged) |
| [MCN-403](MCN-403.md) | GW | MCN-401 |
| [MCN-404](MCN-404.md) | GW | MCN-401 |

## Rulings for the whole sprint

| Ruling | Why | Cost if wrong |
| --- | --- | --- |
| DE 90 correlation format is fixed once, in MCN-401's Ruling 2, and MCN-402 reads a transaction purely from DE 90's four components (original MTI, original STAN, original DE 7, original acquirer ID) — never from any gateway-internal id (`saf_queue.id`, `tran_log.id`) that never crosses the wire | The gateway and issuer are separate services correlating purely over ISO 8583; a correlation scheme that leaked an internal id would be a real protocol bug, not just a style mismatch | MCN-402 built against an assumed correlation key that doesn't match what MCN-401 actually puts on the wire, discovered only at the Sprint 5 integration checkpoint — a full rework of one side |
| `journey.go`'s `StepKind`/`Actor` vocabulary is extended exactly once, in MCN-401's Ruling 3 (`KindReversal = "REVERSAL"`, `ActorSAF = "SAF"`, plus the existing `KindWarn` reused for `REVERSAL_PENDING`) — MCN-403's late-response step and MCN-406's WEB rendering both consume these same values, never inventing parallel ones | Three different stories (401, 403, 406) touch the same journey vocabulary; a second, slightly different set of kind/actor strings introduced by 403 or 406 would silently desync the WEB rendering from what the gateway actually emits | A journey screen that renders `undefined`/fallback styling for a real reversed transaction because the WEB fixture's `kind` string doesn't match the gateway's real one |
| Chaos scenario ids are fixed once by `contracts/openapi.yaml`'s `ChaosScenarioId` enum (`SLOW_NETWORK`, `CONNECTION_CUT`, `DROP_RESPONSE`, `DUPLICATE_REQUEST`, `ISSUER_DOWN`, `LATE_RESPONSE`) — MCN-404's Go consts and MCN-405's i18n keys/MSW fixtures both use these exact six strings, confirmed in each plan's own Ruling | MCN-404 (backend) and MCN-405 (frontend) are built in parallel against MSW mocks with zero cross-communication until the integration checkpoint; a naming drift here is the single most likely cause of that checkpoint failing | The Chaos Lab screen's toggles silently no-op against the real gateway because the ids it sends don't match any of MCN-404's six cases |
| `DROP_RESPONSE`'s real dependency (a fake-issuer response-dropping simulator) does not exist yet and is explicitly scoped to MCN-407 (Sprint 6) — MCN-404 stubs it with a clear `501`, not a silent no-op 200 | Building a duplicate simulator inside MCN-404 to unblock one enum value would either conflict with or duplicate MCN-407's actual work next sprint | Wasted implementation effort on a simulator MCN-407 immediately replaces, or two divergent simulators |
| `saf_queue` and `tran_log.late_response_code`/`late_response_at` are added in **one** migration (MCN-401's `00003_saf_queue.sql`), not split across MCN-401 and MCN-403 | Both stories land in the same sprint and touch the same table; splitting the `ALTER TABLE` into MCN-403's own migration would force MCN-403's worktree to depend on MCN-401's exact migration number, adding merge-order fragility for no benefit | A migration-numbering collision or a MCN-403 branch that can't apply cleanly until MCN-401 merges first, even though the two stories are otherwise independent within Wave 1/2 |

## Sprint 5 exit checklist

- [ ] `make -C gateway-go test` green: `SafRepository` (`ClaimDue` SKIP LOCKED, ack/dead), `saf.Worker` (0420/0421 repeat, backoff, dead-letter), `ReversalQueuer` (atomic timeout/cancellation enqueue), `TestSafWorker_survivesRestart_deliversEveryPendingReversal` (kill -9 durability), `journey` REVERSAL/WARN step tests, `Mux.OnLateResponse`, `RecordLateResponse`, `chaos.ToxiproxyClient`/`Runner`/API handler tests
- [ ] `make -C issuer-jpos test` green: `ReversalWithoutOriginalRepositoryTest`, `LedgerRepositoryReversalTest`, `ParseReversalTest`, `DeduplicateReversalTest`, `LocateAndReverseTest`, `RespondReversalTest`, `ReversalListenerTest`, `ReversalInterleavingPropertyTest` (200-seed randomized interleaving, ledger stays balanced)
- [ ] `make -C web-next lint test build` green: `ScenarioCard`/`MoneyVerificationPanel`/`ReactionPanel`/`ChaosLabScreen`, `StepTimeline`/`MoneyPanel`/`CountdownRing`/`JourneyScreen` (extended), both locales' new `chaos.*`/`journey.countdown.*` keys, Storybook build, a11y checks clean
- [ ] `make contracts` unaffected, still green (no contract changes this sprint)
- [ ] Full integration checkpoint: `make up` with `FF_S4A_REVERSAL=true FF_S4B_CHAOS=true NEXT_PUBLIC_API_MOCKS=false` — pay via the POS simulator with the issuer link cut mid-request (real Toxiproxy `reset_peer`), confirm the transaction reaches `TIMED_OUT` then `REVERSED` without manual intervention, confirm the journey screen renders the WARN/REVERSAL steps and the debit-then-refund money panel against the real gateway, open the Chaos Lab screen against the real backend and run a real 50-transaction chaos run to `PASSED` with `ledgerDiscrepancy: 0`
- [ ] `docs/02` §9 updated if any dependency resolved to an unexpected version
- [ ] Sprint 6 (MCN-407's chaos suite, MCN-501's key rotation) can build directly on this sprint's `saf.Worker`, `journey` vocabulary, and `chaos.Runner` — confirmed no rework needed before Sprint 6 planning starts
