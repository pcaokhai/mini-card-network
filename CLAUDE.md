# CLAUDE.md — Mini Card Network

Production-patterned card payment network built to learn ISO 8583 end to end.
Next.js POS + ops console → Go acquirer gateway → (Toxiproxy) → jPOS issuer; issuer events → Kafka → Spring Boot settlement.

This file is loaded into every session. Keep it short. Detailed rules live in `docs/`; read the doc that matches your task before you plan.

## 1. Repository map

```
issuer-jpos/   Java 25 · jPOS Q2 · Gradle     Issuer host (ISO 8583 server) + Issuer Admin REST API     → issuer-jpos/CLAUDE.md
gateway-go/    Go                              Acquirer gateway: REST/WS API, codec, MUX, SAF, switch    → gateway-go/CLAUDE.md
web-next/      Next.js App Router · TS strict  POS simulator + operations console (BFF)                  → web-next/CLAUDE.md
settlement/    Java 25 · Spring Boot           Outbox consumer, reconciliation, clearing file            → settlement/CLAUDE.md
contracts/     Source of truth for every interface: openapi.yaml, ws-events.schema.json, iso8583/
infra/         docker-compose, Postgres, Kafka, Toxiproxy, OpenTelemetry, Prometheus, Grafana
docs/          PRD, architecture, ISO spec, API, data model, stories, delivery plan, tests, risks, standards, ADRs
docs/plans/    Implementation plans written by the writing-plans skill (one file per story)
```

## 2. Which doc to read

| You are about to… | Read first |
| --- | --- |
| Start any story | `docs/06-user-stories.md` (the story + AC) and `docs/07-delivery-plan.md` (lane, slice, dependencies) |
| Touch ISO 8583 messages, fields, timers, codes | `docs/03-iso8583-interface-spec.md` |
| Touch REST or WebSocket endpoints | `contracts/openapi.yaml`, `contracts/ws-events.schema.json`, `docs/04-api-contract.md` |
| Touch tables or migrations | `docs/05-data-model.md` |
| Make a design choice | `docs/02-software-architecture.md` and `docs/adr/` |
| Write tests | `docs/08-test-strategy.md` |
| Write any code | `docs/10-engineering-standards.md` (full rules; summary in §6 below) |

Precedence when documents disagree: accepted ADR > `contracts/` > ISO spec > architecture > user stories > this file. Record the conflict as a Ruling in the plan and open a doc-fix PR.

## 3. Commands (scaffolded by MCN-001; keep them working)

```bash
make up            # start infra + all services (docker compose)
make down
make test          # all unit + integration tests, every service
make lint          # all linters + formatters in check mode
make fmt           # auto-format everything
make contracts     # lint OpenAPI, validate WS schema, run ISO golden vectors against both codecs, check breaking changes
make e2e           # Playwright journeys against the running stack
make chaos         # chaos suite with ledger invariant check (MCN-407)
make pci-scan      # scan logs and fixtures for PAN / PIN block / track data (MCN-506)
```

Per-service commands are in each service's `CLAUDE.md`.

## 4. How we work — Superpowers workflow (mandatory)

Every story follows this sequence. Do not skip steps; do not reorder.

1. **brainstorming** — only when the story leaves a design decision open. If the docs already decide it, write a one-paragraph understanding that cites the doc sections and move on.
2. **using-git-worktrees** — one worktree and branch per story: `feat/MCN-<id>-<slug>` (or `fix/`, `chore/`). Worktrees live in `../mcn-worktrees/`.
3. **writing-plans** — save to `docs/plans/MCN-<id>.md`. Tasks are 2–5 minutes, name exact files, include the failing test first and the verification command. Keep plans lean: interfaces (function/type names and signatures), test names and their expected assertions in prose, Rulings, and the AC table — not full illustrative implementation code. Sprint 3–4 found that hand-written code drafts in plans were consistently rewritten from scratch by the implementer after reading the real merged codebase anyway (interfaces drift as stories merge in parallel); writing them cost real planning-time tokens without improving the result.
4. **subagent-driven-development** (default) or **executing-plans** (small or tightly coupled stories).
5. **test-driven-development** — RED → GREEN → REFACTOR for every behavior change. No production code without a failing test first.
6. **verification-before-completion** — run the verification commands and paste real output before saying "done", "fixed" or "passing".
7. **Review, solo-project shape.** This is a solo-operated lab repo with no human reviewer and no PR-review branch protection — a human `requesting-code-review`/`receiving-code-review` cycle does not happen. What actually substitutes: the orchestrating session independently rebuilds and re-runs the real verification commands after every rebase, before merging, never trusting a subagent's self-report alone. For a security-sensitive change (auth, PAN handling, key material, the ledger), explicitly dispatch a **code-reviewer** agent against the diff before merge — don't skip that for those cases the way ordinary stories skip human review.
8. **finishing-a-development-branch** — rebase on `main`, green CI, squash-merge with a Conventional Commit title.

Bugs and failing tests: **systematic-debugging** first. Reproduce, find the root cause, add a regression test, then fix. Never guess-fix.

This project does not require the global workflow's "Research & Reuse" step (GitHub/package-registry search before implementing) — across 20 stories in Sprints 0–4 it was never once performed and never once missed: this codebase's logic (ISO 8583 framing, the authorization chain, the ledger) is domain-specific enough that there's rarely an existing implementation to fork, and the one place genuine reuse matters — both ISO 8583 codecs generated from one shared `contracts/iso8583/packager-spec.yaml` — is already covered by ADR-003, not by a per-story search step. Go straight from the story's AC to `writing-plans`.

## 5. Parallel work rules

- **Contract first.** Any change that crosses a boundary (REST, WebSocket, ISO 8583, Kafka event, DB owned by another service) starts with a small PR to `contracts/` that both sides approve. Provider and consumer then build in parallel against it.
- **Feature slices ship together.** A slice = the backend story + the frontend story that share one contract. Develop them in parallel worktrees, merge both, release together behind the slice's feature flag. See the slice table in `docs/07-delivery-plan.md`.
- **Lanes own directories.** ISS = `issuer-jpos/`, GW = `gateway-go/`, WEB = `web-next/`, SET = `settlement/`, PLAT = `infra/`, `contracts/`, CI. Use **dispatching-parallel-agents** only for tasks whose file sets do not overlap. Two agents never edit the same file, migration or contract in the same wave.
- **Frontend never waits for backend.** WEB builds against MSW mocks generated from `contracts/openapi.yaml` and fixtures in `contracts/fixtures/`. Switching to the real backend is a config flag, not a code change.
- **Migrations** are owned by one service each. Reserve the next migration number in the plan before writing it.
- **Integration checkpoint** at the end of each slice: `make up && make e2e` on `main` with the flag on.
- **Cap parallel dispatch at 2–3 subagents per wave.** Sprint 4 dispatched 3–4 at once and all hit the session usage limit simultaneously, losing the whole wave to a reset wait. Stagger dispatch or keep the wave smaller instead.
- **Dispatch prompts never ask a subagent to run `gh pr merge`.** It is always denied by the auto-mode permission classifier ("Merge Without Review") — confirmed across every PR in Sprints 2–4, no exceptions. Instruct subagents to stop once CI is confirmed green and report back; the orchestrating session merges.
- **Verify CI with `gh run list --branch <branch>` alongside `gh pr checks`, every time.** Twice in Sprint 4 a PR showed only `GitGuardian` passing (no real GitHub Actions run at all) while `gh pr checks`'s summary still looked plausible — the real cause was a stale branch needing a rebase. Don't trust the checks summary alone.
- **Cleanup order: `git worktree remove` before `gh pr merge --delete-branch`.** The reverse order fails every time ("cannot delete branch ... used by worktree") and costs an extra round trip.
- **Poll CI in the background**, not with a blocking shell loop — frees the turn to prep the next wave's worktrees while waiting.

## 6. Engineering rules (summary — full text in `docs/10-engineering-standards.md`)

Non-negotiable. A violation blocks merge.

1. **Money** is integer minor units: `long` (Java), `int64` (Go), `BIGINT` (SQL), integer in JSON. Never float, double or BigDecimal for amounts. Currency is ISO 4217 numeric (`"704"`).
2. **PCI DSS.** Never log, persist in clear, return or put in URLs: full PAN, track 2, CVV, PIN, PIN block, clear keys. Display PAN as first 6 + last 4 at most. Use the shared masker; every new log line with card data needs a masking test.
3. **Idempotency.** Every state-changing REST call takes `Idempotency-Key`. Every ISO request is deduplicated on (acquirer ID, TID, STAN, field 7, MTI class, business date).
4. **Unknown outcome ⇒ reversal.** After a timeout or broken connection, never resend the 0200. Queue a 0420 in SAF and let the worker deliver it.
5. **Double-entry ledger.** Balances change only through journal entries whose debits equal credits, inside the same DB transaction as the tran_log row.
6. **Architecture.** Hexagonal: domain has no framework or I/O imports; adapters depend on ports, never the reverse. Enforced by ArchUnit (Java) and import rules (Go, TS).
7. **SOLID and patterns.** Small single-purpose units; extend by adding (new participant, new strategy), not by editing switch statements. Use the patterns named in the standards for the named problems; do not invent new abstractions for one use.
8. **Errors.** Typed domain errors mapped to ISO response codes and HTTP problem details in exactly one place per service. No swallowed errors, no `catch (Exception)` without rethrow or explicit handling.
9. **Concurrency.** Every goroutine/thread has an owner and a cancellation path. Account updates use row locks or optimistic versions as documented; never read-modify-write without one.
10. **Observability.** Structured JSON logs with `trace_id`, `rrn`, `stan`, `mti`, `rc`; W3C `traceparent` propagated across REST, WS and Kafka headers. ISO 8583 carries no trace header: both hosts log the trace id against (STAN, field 7, RRN) as described in ISO spec §10, and never add private fields for tracing.
11. **Config** through environment variables with typed validation at startup; no secrets in the repo; fail fast on invalid config.
12. **Readable code.** Intention-revealing names from the glossary, functions under ~30 lines, no comments that restate code, comments explain *why*. Public APIs documented.
13. **Tests.** Unit tests for domain logic, integration tests with Testcontainers for adapters, contract tests at every boundary, golden vectors for ISO. Coverage gates in `docs/08-test-strategy.md`. No `@Disabled` / `t.Skip` / `.skip` without an issue link.
14. **PR hygiene.** One story per PR, Conventional Commits, PR template filled. No hard line-count cap: every Sprint 3–4 PR ran 700–1700 changed lines (a subagent builds a whole story — schema, repositories, handlers, tests — in one pass) and none were split further without harm. Split a PR only when it genuinely crosses more than one lane/service, not to hit a line-count number.
15. **Generated code** (OpenAPI clients/servers, sqlc, jOOQ if adopted) is never edited by hand. Change the source and regenerate.

## 7. Definition of Done (per story)

- [ ] All acceptance criteria demonstrated by automated tests that reference the AC id (e.g. `MCN-303-AC2`)
- [ ] `make lint test contracts` green locally and in CI
- [ ] No new PCI scanner findings; masking tests for new card-data paths
- [ ] Metrics/logs/traces added for new flows as listed in the story
- [ ] Docs updated in the same PR when behavior, contract or schema changes (ISO spec, API doc, data model, ADR)
- [ ] For slice stories: the paired FE/BE story is merged or merging in the same release, flag documented
- [ ] Code review completed and all blocking comments resolved

## 8. Domain glossary (use these names in code)

acquirer, issuer, switch, terminal (TID), merchant (MID), PAN, STAN, RRN, MTI, bitmap, field/DE, response code (RC), authorization, financial request, advice, repeat, reversal, SAF (store-and-forward), MUX, echo, sign-on, cutover, business date, reconciliation (0500), clearing, net position, hold (auth hold), completion, STIP (stand-in processing), velocity, KCV, LMK/ZMK/ZPK/ZAK/TPK/TAK/CVK/PVK, PIN block, MAC, ARQC/ARPC, TLV.

## 9. Things Claude must not do

- Do not change `contracts/` inside a feature branch that also changes a service; contract PRs are separate.
- Do not add a dependency without noting it in the plan with its purpose and license.
- Do not weaken a test, a lint rule or a coverage gate to make CI pass.
- Do not use real card numbers. Use the BIN `970436` test cards from `contracts/fixtures/cards.json` only.
- Do not bypass the SAF, the ledger or the masker, even in tests of other features — use the test helpers.
