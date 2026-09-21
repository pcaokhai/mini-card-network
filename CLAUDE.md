# CLAUDE.md — Mini Card Network

Production-patterned card payment network built to learn ISO 8583 end to end.
Next.js POS + ops console → Go acquirer gateway → (Toxiproxy) → jPOS issuer; issuer events → Kafka → Spring Boot settlement.

This file is loaded into every session. Keep it short. Detailed rules live in `docs/`; read the doc that matches your task before you plan.

## 1. Repository map

```
issuer-jpos/   Java 21 · jPOS Q2 · Gradle     Issuer host (ISO 8583 server) + Issuer Admin REST API     → issuer-jpos/CLAUDE.md
gateway-go/    Go                              Acquirer gateway: REST/WS API, codec, MUX, SAF, switch    → gateway-go/CLAUDE.md
web-next/      Next.js App Router · TS strict  POS simulator + operations console (BFF)                  → web-next/CLAUDE.md
settlement/    Java 21 · Spring Boot           Outbox consumer, reconciliation, clearing file            → settlement/CLAUDE.md
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
3. **writing-plans** — save to `docs/plans/MCN-<id>.md`. Tasks are 2–5 minutes, name exact files, include the failing test first and the verification command.
4. **subagent-driven-development** (default) or **executing-plans** (small or tightly coupled stories).
5. **test-driven-development** — RED → GREEN → REFACTOR for every behavior change. No production code without a failing test first.
6. **verification-before-completion** — run the verification commands and paste real output before saying "done", "fixed" or "passing".
7. **requesting-code-review** → **receiving-code-review** — review against §6, the story AC and the ISO spec.
8. **finishing-a-development-branch** — rebase on `main`, green CI, squash-merge with a Conventional Commit title.

Bugs and failing tests: **systematic-debugging** first. Reproduce, find the root cause, add a regression test, then fix. Never guess-fix.

## 5. Parallel work rules

- **Contract first.** Any change that crosses a boundary (REST, WebSocket, ISO 8583, Kafka event, DB owned by another service) starts with a small PR to `contracts/` that both sides approve. Provider and consumer then build in parallel against it.
- **Feature slices ship together.** A slice = the backend story + the frontend story that share one contract. Develop them in parallel worktrees, merge both, release together behind the slice's feature flag. See the slice table in `docs/07-delivery-plan.md`.
- **Lanes own directories.** ISS = `issuer-jpos/`, GW = `gateway-go/`, WEB = `web-next/`, SET = `settlement/`, PLAT = `infra/`, `contracts/`, CI. Use **dispatching-parallel-agents** only for tasks whose file sets do not overlap. Two agents never edit the same file, migration or contract in the same wave.
- **Frontend never waits for backend.** WEB builds against MSW mocks generated from `contracts/openapi.yaml` and fixtures in `contracts/fixtures/`. Switching to the real backend is a config flag, not a code change.
- **Migrations** are owned by one service each. Reserve the next migration number in the plan before writing it.
- **Integration checkpoint** at the end of each slice: `make up && make e2e` on `main` with the flag on.

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
14. **PR hygiene.** One story per PR, under ~400 changed lines excluding generated code, Conventional Commits, PR template filled.
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
