# Sprint 3 — Execution Overview

> **For Claude:** This file orchestrates Sprint 3. Each story has its own plan (`MCN-30x.md`). Execute each plan with superpowers:subagent-driven-development (or superpowers:executing-plans), one worktree per story, following root `CLAUDE.md` §4. Contract-first check: `contracts/openapi.yaml` (`transactions/purchases`, `terminals`, `metrics/overview`) and `contracts/ws-events.schema.json` (`transaction.created`/`transaction.updated`) already have every shape this sprint needs from the original doc pack — **no new contract PR is needed before Wave 1 starts.**

**Sprint goal:** First purchase flows work end to end on mocks and against the real issuer: the issuer has a real schema, seed data, and an authorization chain that correctly declines every non-approval case; the gateway builds and sends a real purchase; the POS simulator and overview dashboard exist on the frontend. **Duration:** 2 weeks. **Commitment:** 3 (MCN-301) + 5 (MCN-302a) + 8 (MCN-303) + 8 (MCN-305) + 5 (MCN-306) = 29 points. **Release:** — (per `docs/07-delivery-plan.md`, R3 ships in Sprint 4 once MCN-302b/304/307/308/309 also land; this sprint's slices merge but their feature flags stay off until the full slice is done).

## Waves

| Wave | Stories (parallel lanes) | Starts when |
| --- | --- | --- |
| 1 | ISS **MCN-301** schema/seed → **MCN-302a** validate/decline chain (sequential, same lane) ‖ GW **MCN-303** purchase flow ‖ WEB **MCN-305** POS simulator ‖ WEB **MCN-306** overview dashboard | Sprint 2 merged |

MCN-301 → MCN-302a are sequential within the ISS lane (302a needs the schema and `CardRepository` from 301). MCN-303 (GW) needs only MCN-302's **contract** (already in `contracts/openapi.yaml`/ISO spec), not its merged code — it builds and tests against an in-process fake issuer, the same pattern MCN-202/203 used, and only needs the real issuer at the Sprint 3→4 integration checkpoint. MCN-305 and MCN-306 (WEB) build against MSW mocks and depend on neither ISS nor GW, per the delivery plan's "frontend never waits for backend" rule already proven in Sprints 1–2.

This means all four lanes can start in parallel on day 1: ISS on MCN-301, GW on MCN-303, WEB on MCN-305 and MCN-306 (two WEB stories, run as two separate worktrees/subagents since they touch disjoint files — `pos/` vs `overview/` — with one shared new file, `useWsEvents.ts`, owned by MCN-306; MCN-305 does not touch it).

## Plans

| Plan | Lane | Depends on |
| --- | --- | --- |
| [MCN-301](MCN-301.md) | ISS | Sprint 2 / MCN-201 (merged) |
| [MCN-302a](MCN-302a.md) | ISS | MCN-301 |
| [MCN-303](MCN-303.md) | GW | Sprint 2 / MCN-203 (merged); MCN-302 contract only |
| [MCN-305](MCN-305.md) | WEB | Sprint 0 / MCN-004 (merged) |
| [MCN-306](MCN-306.md) | WEB | Sprint 0 / MCN-004 (merged) |

**MCN-302b (authorize/ledger, 8 pts) is a separate story/plan not written this sprint** — per `docs/07-delivery-plan.md`'s Sprint 4 wave table, it's Sprint 4 work (`ISS MCN-302b`), written and implemented after this sprint's plans are done. MCN-302a's own plan documents exactly what it stubs (RC 96 placeholder instead of a real RC 00 approval) so 302b has an unambiguous starting point.

## Rulings for the whole sprint

| Ruling | Why | Cost if wrong |
| --- | --- | --- |
| `docs/assets/baseline-schema.sql` (not just `docs/05-data-model.md`'s prose) is the real DDL source of truth for MCN-301/303's migrations | The actual baseline file exists and has every column; inventing schema again (as MCN-201 had to, before this file was found) would create yet another reconciliation debt | A schema drift between what MCN-301 writes and what MCN-302a/303 expect, caught late |
| MCN-201's already-shipped `acquirer_link` table is reconciled (add `name`, rename `last_sign_on_at`→`signed_on_at`) rather than replaced, to avoid breaking live, tested code | MCN-201 predates the discovery of the baseline DDL file; its schema works and is covered by real integration tests | Rewriting it risks a regression in already-merged, working network-management code for a purely cosmetic schema-purity win |
| MCN-302's 13 points split at 302a (validate/decline, RC ≠ 00/51) / 302b (approve, ledger, concurrency, perf) exactly as the user story and delivery plan specify | Matches the story's own point split and the Sprint 3/4 wave assignment; keeps 302a mergeable without inventing money-movement code ahead of its dedicated story | If blurred, 302a's PR either grows unreviewably large or ships incomplete/untested ledger code |
| MCN-303 resolves `cardToken → PAN` from `contracts/fixtures/cards.json` directly (embedded at build time), never accepting a raw PAN over the wire | `CardPresentData.cardToken` is explicitly documented as "never a PAN"; the gateway needs the real PAN only momentarily to build DE 2 | If skipped, either PII (a real-looking PAN) crosses the API boundary, or purchases can't build a valid ISO message at all |
| MCN-305's simulator PIN-block "encryption" uses a fixed, documented, non-secret simulator TPK, and does not attempt byte-for-byte correctness against the gateway-resolved real PAN | A browser can never safely hold a production key; demonstrating the *shape* of ISO 9564 PIN-block building satisfies the story's teaching goal without shipping real PANs client-side | None if understood as a lab simplification; would be a real vulnerability if mistaken for production-grade PIN security |
| MCN-306 introduces the first `useWsEvents` hook in `web-next/`, but does not migrate MCN-205's screen from polling to it in the same story | Keeps this story's diff scoped to what it actually needs (the live feed); the migration is a valuable but separate, low-risk follow-up | None — MCN-205 keeps working exactly as before until someone does the follow-up |
| Overview KPIs (MCN-306) refresh on a 30s timer, not via WS push; only the live feed is WS-driven | KPIs are point-in-time aggregates, not per-event state — pushing a full KPI recompute on every transaction would be wasted work for a lab-scale dashboard (YAGNI) | If traffic volume ever demanded live KPIs, a later story adds a dedicated `metrics.changed` WS event — not needed yet |

## Sprint 3 exit checklist

- [ ] `make -C issuer-jpos test` green: `SchemaMigrationTest`, `CardCryptoTest`, `CardRepositoryTest`, `SeedLoaderTest`, `TranLogRepositoryTest`, every participant unit test, `PurchaseDeclineIntegrationTest` (real socket, real chain, real Postgres)
- [ ] `make -C gateway-go test` green: `cardtokens`, `rrn`, `TranLogRepository`/`IdempotencyRepository` (Testcontainers), `purchase.Service` unit tests, `purchases` handler tests
- [ ] `make -C web-next lint test build` green: `pinblock`, `Keypad`/`PinPad`/`CardPicker`/`ScenarioButtons`/`ResultPanel`/`PosScreen`, `useWsEvents`/`KpiCards`/`ThroughputChart`/`DeclineReasonsBreakdown`/`SystemHealthList`/`CutoverCountdown`/`LiveFeed`/`OverviewScreen`, both locales' `pos.*`/`overview.*` keys, Storybook build, a11y checks clean
- [ ] `make contracts` unaffected, still green
- [ ] Integration checkpoint (partial this sprint — full checkpoint waits for Sprint 4's MCN-302b/304): with `make up` and the real issuer/gateway, a purchase against `tok_normal` with an in-limits amount currently returns RC 96 ("ledger not implemented until MCN-302b") by design — confirm this is the *only* unexpected-looking result, not a real bug, before moving to Sprint 4
- [ ] `docs/02` §9 updated if any dependency resolved to an unexpected version
