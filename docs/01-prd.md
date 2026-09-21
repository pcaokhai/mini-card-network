# PRD — Mini Card Network

Version 1.0 · 2026-09-21

## 1. Summary

Mini Card Network is a small but production-patterned card payment network that runs on one laptop. It lets an engineer learn ISO 8583 and the patterns real payment systems rely on (idempotency, reversal, store-and-forward, double-entry ledger, key management, reconciliation, stand-in) by building them, breaking them on purpose, and watching them recover. A web console makes every step visible to technical and non-technical viewers.

## 2. Contacts

| Name / role | Responsibility | Comment |
| --- | --- | --- |
| Khai — Product Owner and Tech Lead | Scope, priorities, architecture decisions, final review | Owns this PRD, ADRs and `contracts/` |
| ISS lane (Java/jPOS engineer or Claude Code session) | Issuer host and Issuer Admin API | See `issuer-jpos/CLAUDE.md` |
| GW lane (Go engineer or Claude Code session) | Acquirer gateway, codec, SAF, switch | See `gateway-go/CLAUDE.md` |
| WEB lane (frontend engineer or Claude Code session) | POS simulator and operations console | See `web-next/CLAUDE.md` |
| SET lane (Java/Spring engineer or Claude Code session) | Settlement service | See `settlement/CLAUDE.md` |
| PLAT lane (Tech Lead) | Infra, CI, contracts, observability | Reviews every contract PR |

## 3. Background

**Context.** Card payments still run largely on ISO 8583. The standard is simple to read and hard to operate: most production incidents come from timeouts, duplicates, late responses, key changes and end-of-day mismatches, not from parsing. Public material explains fields; very little lets you practise the failure handling.

**Why now.** The product owner has fintech experience (PCI DSS, ISO 8583) and wants a deep, demonstrable portfolio project for senior backend and fintech roles, plus a reusable lab for teaching. Mature open-source building blocks (jPOS, Go ISO libraries, Toxiproxy, OpenTelemetry) and AI coding agents make a one-person build of a multi-service network realistic within about 22 weeks.

## 4. Objective

Build a network that behaves like production under failure, and explain it clearly.

| Objective | Key result (measurable) |
| --- | --- |
| O1. Correct under failure | KR1: 10,000 transactions under all six chaos scenarios end with ledger = tran_log to the last đồng (zero discrepancy), checked automatically. KR2: zero lost advices after `kill -9` of the gateway during load. |
| O2. Production-grade engineering | KR3: every boundary covered by contract tests; CI gates green on `main` for 90% of days. KR4: p99 end-to-end < 300 ms at 200 TPS on a laptop. |
| O3. Understandable by anyone | KR5: a non-technical viewer can explain what happened in a timeout-and-reversal journey after one walkthrough (tested with 3 people). KR6: all 9 screens work in Easy and Expert modes. |
| O4. Portfolio value | KR7: 10 ADRs and 10 short blog posts, one per phase; a 5-minute demo video. |

## 5. Market segments

Segments are defined by the job to be done:

- **Engineers learning payment systems** — need to see and break real flows, not read field tables. Constraint: one laptop, no access to scheme specifications or real HSMs.
- **Interviewers and reviewers** — need to judge seniority quickly. Constraint: 5–10 minutes of attention; need clear architecture, tests and decisions.
- **Non-technical stakeholders** (product, ops, business) — need to understand why payments fail and what "reversal" or "reconciliation" means. Constraint: no technical vocabulary.

## 6. Value propositions

| Job | Gain | Pain avoided | Better than alternatives because |
| --- | --- | --- | --- |
| Understand ISO 8583 deeply | Byte-level Message Lab with bitmap grid and field explanations | Memorising field tables | Every field is live, editable and tied to real flows |
| Practise failure handling | Chaos Lab + money verification after every run | Learning incident handling in production | Failures are one click and the ledger invariant is proven each time |
| Show senior-level work | Hexagonal services, contracts, ADRs, tests, observability | Toy projects without operations depth | Mirrors how real processors are built and run |
| Explain payments to non-engineers | Easy mode, journey playback, plain-language glossary | Jargon-heavy dashboards | Same screen serves both audiences with one toggle |

## 7. Solution

### 7.1 UX and prototypes

Interactive design of all nine screens (Overview, POS, Transaction Journey, Message Lab, Chaos Lab, Cards and Accounts, Security and Keys, Settlement, Network Operations): **https://claude.ai/artifact/LUUvgVcXYjqkHwdRui1Jmw**. The design includes the Easy/Expert toggle and the motion system. Screen-by-screen behavior is specified in the user stories (`06-user-stories.md`).

### 7.2 Key features

| Feature | Description | Epic |
| --- | --- | --- |
| ISO 8583 codec and Message Lab | Own Go codec (ASCII packager, primary/secondary bitmap, LL/LLL fields) cross-checked with jPOS; decode/encode UI | E1 |
| Host-to-host connectivity | Sign-on, echo, reconnect with backoff, MUX matching, network operations screen | E2 |
| Authorization and POS | 0100/0200 flows, limits, holds, double-entry ledger, POS simulator, live overview, journey view, card admin | E3 |
| Resilience | Timeout → reversal, SAF with repeats, duplicate replay, late-response handling, chaos lab with money verification | E4 |
| Security | Key hierarchy with KCV, PIN translation and verification, MAC, dynamic key exchange, PCI log scanning | E5 |
| EMV and advanced transactions | Field 55 TLV, ARQC/ARPC simulation, pre-auth/completion, refund, balance inquiry, partial approval | E6 |
| Settlement | Cutover, 0500 totals, event-driven clearing, reconciliation breaks, net position, clearing file | E7 |
| Switch and stand-in | BIN routing, circuit breaker, STIP with advices, velocity rules | E8 |
| Operations and learning | Dashboards, SLO alerts, runbooks, graceful drain, load test, lesson mode, ⌘K search, English UI | E9 |

Functional (FR-01…FR-18) and non-functional (NFR-01…NFR-10) requirements are listed in `02-software-architecture.md` §2 and traced to stories in `06-user-stories.md`.

### 7.3 Technology

Java 25 + jPOS (issuer), Go (gateway, switch), Next.js + TypeScript (web), Java 25 + Spring Boot (settlement), PostgreSQL, Kafka, Toxiproxy, OpenTelemetry, Prometheus, Grafana, Docker Compose. Details and rationale: `02-software-architecture.md` and ADRs.

### 7.4 Assumptions (to validate)

| ID | Assumption | How we validate | By |
| --- | --- | --- | --- |
| A1 | A software HSM (jPOS JCESecurityModule + Go simulator) is realistic enough to teach key handling | Compare flows with public HSM command references in Sprint 6 | MCN-501 |
| A2 | 200 TPS is reachable on a laptop with Docker | Load test spike in Sprint 4 on the authorization path | MCN-303 |
| A3 | Contract-first with MSW mocks lets WEB run fully in parallel with BE | Track days WEB is blocked by BE per sprint (target 0) | Every retro |
| A4 | Non-technical viewers understand Easy mode without narration | Hallway test with 3 people at R3 and R4 | R3, R4 |
| A5 | Claude Code lanes can run concurrently without merge conflicts when lanes own directories | Count cross-lane conflicts per sprint (target ≤ 1) | Every retro |

## 8. Release

Relative timeframes; one sprint = 2 weeks except Sprint 0 (1 week). Each release ships backend and frontend of its slices together behind feature flags.

| Release | Content | When |
| --- | --- | --- |
| R0 Foundation | Monorepo, CI, infra, contracts v0, service skeletons, web shell | End of Sprint 0 |
| R1 Message Lab | Codec + Message Lab screen | End of Sprint 1 |
| R2 Connectivity | Sign-on, echo, MUX, Network screen v1 | End of Sprint 2 |
| R3 Authorization | Purchase and auth end to end, POS, Overview, Journey, Cards | End of Sprint 4 |
| R4 Resilience | Reversal, SAF, dedupe, Chaos Lab | End of Sprint 6 |
| R5 Security | Keys, PIN, MAC, key exchange, Security screen | End of Sprint 7 |
| R6 Advanced transactions | EMV, pre-auth/completion, refund, balance | End of Sprint 8 |
| R7 Settlement | Cutover, 0500, recon, clearing file, Settlement screen | End of Sprint 9 |
| R8 Switch and STIP | Routing, circuit breaker, STIP, velocity, Network v2 | End of Sprint 10 |
| R9 Operations (v1.0) | Dashboards, alerts, runbooks, load test, lesson mode, E2E suite | End of Sprint 11 |

**Out of scope for v1.0:** real scheme certification, ISO 20022, 3-D Secure, tokenization, chargebacks, multi-currency conversion, cloud deployment. These are candidates for v2.
