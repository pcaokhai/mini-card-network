# Sprint 4 — Execution Overview

> **For Claude:** This file orchestrates Sprint 4. Each story has its own plan (`MCN-30x.md`). Execute each plan with superpowers:subagent-driven-development (or superpowers:executing-plans), one worktree per story, following root `CLAUDE.md` §4. Contract-first check: `contracts/openapi.yaml` (`/v1/transactions`, `/v1/transactions/{rrn}`, `/v1/transactions/{rrn}/journey`, `/v1/cards*`) already has every shape this sprint needs from the original doc pack — **no new contract PR is needed before Wave 1 starts.**

**Sprint goal:** Purchase, overview, journey, and cards are all released together as R3: real approval with a double-entry ledger that survives concurrency, transaction history and journey replay, and card/account administration. **Duration:** 2 weeks. **Commitment:** 8 (MCN-302b) + 5 (MCN-307) + 5 (MCN-309) + 5 (MCN-304) + 5 (MCN-308) = 28 points. **Release:** R3.

## Waves

| Wave | Stories (parallel lanes) | Starts when |
| --- | --- | --- |
| 1 | ISS **MCN-302b** approve/ledger ‖ WEB **MCN-307** Transaction Journey screen ‖ WEB **MCN-309** Cards and Accounts screen | Sprint 3 merged |
| 2 | GW **MCN-304** transactions/journey API ‖ ISS **MCN-308** Admin API | Wave 1 merged (MCN-308 needs MCN-302b's ledger repositories in the same lane; MCN-304 is placed here per the delivery plan's own wave table even though its only hard dependency, MCN-303, already merged in Sprint 3 — followed as written, matching how every prior sprint has treated `docs/07-delivery-plan.md`'s wave assignment as authoritative even when a story's own dependency list would technically allow earlier parallelism) |

MCN-307 and MCN-309 (WEB) build against MSW mocks and do not wait for MCN-304/MCN-308, per the delivery plan's "frontend never waits for backend" rule proven in every prior sprint. Both make small, additive edits to already-merged Sprint 3 WEB files (`LiveFeed.tsx`, `ResultPanel.tsx` for MCN-307's cross-screen navigation links) — confirmed non-overlapping with each other's own new files.

## Plans

| Plan | Lane | Depends on |
| --- | --- | --- |
| [MCN-302b](MCN-302b.md) | ISS | Sprint 3 / MCN-302a (merged) |
| [MCN-307](MCN-307.md) | WEB | Sprint 0 / MCN-004 (merged); Sprint 3 / MCN-305, MCN-306 (merged, for the small nav-link edits) |
| [MCN-309](MCN-309.md) | WEB | Sprint 0 / MCN-004 (merged) |
| [MCN-304](MCN-304.md) | GW | Sprint 3 / MCN-303 (merged) |
| [MCN-308](MCN-308.md) | ISS | MCN-302b |

## Rulings for the whole sprint

| Ruling | Why | Cost if wrong |
| --- | --- | --- |
| MCN-302b's row lock + debit + journal-post all share one explicit JDBC transaction, confirmed against jPOS's real (lack of) built-in connection-sharing across `TransactionParticipant`s before writing any code | A lock taken on a connection that closes before the debit commits is a real correctness bug, not a style choice — this is the single most safety-critical piece of code in the project so far | A silent race that lets `chk_available_floor` or the ledger-balance trigger be the only thing standing between correct and incorrect money movement |
| `AuthorizationListener`'s already-discovered `AbortParticipant` requirement (MCN-302a) extends to `Authorize` itself in MCN-302b if it needs any cleanup on a decline path it doesn't already handle via its own explicit transaction rollback | jPOS skips plain (non-`AbortParticipant`) participants' `abort()` entirely once a transaction is known to fail — already bit MCN-302a once; the fix pattern is now known and must be reapplied wherever relevant | A silent no-op cleanup path that looks correct until a real abort scenario is hit in production |
| `card_ref` (MCN-308) is the only card identifier ever exposed in an API response — the internal `BIGSERIAL id` never crosses the wire, per `docs/05-data-model.md` §3's opaque-external-reference rule | Matches the modeling convention every other entity in this schema already follows (RRN, cardToken) | Exposing an internal auto-increment ID leaks row-count/ordering information and makes IDOR-style enumeration trivial |
| MCN-307/MCN-309 confirm the real ETag/header-capture mechanism their generated `openapi-fetch` client actually supports before writing any mutation code, rather than assuming a header round-trips through the typed wrapper by default | `openapi-fetch`'s typed convenience wrapper commonly drops raw response headers; this has to be checked against the real installed version, not assumed | A silently-broken `If-Match` flow that always sends a stale or missing header, making every 412 test either pass for the wrong reason or never exercise the real path |
| MCN-304 designs a genuinely new keyset cursor convention if MCN-204's `/v1/network/events` turns out (on inspection) not to have shipped one yet, rather than inventing a second, incompatible pagination scheme later | Two different cursor formats across the same API would be a real API-consistency bug a learner would notice immediately | A confusing, non-uniform pagination experience across `/v1/network/events` and `/v1/transactions` |

## Sprint 4 exit checklist

- [ ] `make -C issuer-jpos test` green: `AccountLockRepositoryTest`, `LedgerRepositoryTest`, `AuthCodeGeneratorTest`, updated `AuthorizeTest`, `PurchaseApprovalIntegrationTest`, `ConcurrentPurchaseLoadTest` (200 concurrent purchases, ledger invariant holds, p99 < 100ms), `V4` migration test, `CardLimitRepositoryTest`/`AuditLogRepositoryTest`/`IdempotencyRepositoryTest`, `CardAdminApiIntegrationTest`
- [ ] `make -C gateway-go test` green: `TranLogRepository.List`/`.Get`, `internal/journey` builder tests, transactions query handler tests
- [ ] `make -C web-next lint test build` green: `PlaybackControls`/`StepTimeline`/`SummaryHeader`/`StepDetail`/`MoneyPanel`/`JourneyScreen`, `CardVisual`/`BalanceLines`/`HoldsList`/`LimitsSliders`/`BlockConfirmDialog`/`AuditLine`/`LedgerTable`/`CardsListScreen`/`CardDetailScreen`, both locales' `journey.*`/`cards.*` keys, Storybook build, a11y checks clean
- [ ] `make contracts` unaffected, still green
- [ ] Full integration checkpoint (the one deferred from Sprint 3): `make up`, real issuer + real gateway, `NEXT_PUBLIC_API_MOCKS=false` — pay via the POS simulator, confirm a real RC 00 approval with a real auth code, open the resulting transaction's journey and see real steps, open Cards and Accounts and see the real post-purchase balance change, block a card and confirm a real audit log entry
- [ ] `docs/02` §9 updated if any dependency resolved to an unexpected version
- [ ] `docs/releases/R3.md` written once the checkpoint passes, covering all of Sprint 3 + Sprint 4's stories together (MCN-301 through MCN-309, since R3 is their combined release per `docs/07-delivery-plan.md`)
