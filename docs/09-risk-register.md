# Risk Register — Pre-mortem

Version 1.0 · 2026-09-21 · Owner: Tech Lead · Review: every retro; full re-run 2 sprints before R3, R4 and R9

Exercise: "It is the end of Sprint 11 and v1.0 failed: the demo breaks, the ledger drifts under chaos, and the codebase is hard to explain in an interview. What happened?"

## Tigers (real risks)

| ID | Risk | Urgency | Mitigation | Owner | Check by |
| --- | --- | --- | --- | --- | --- |
| R-01 | Review bottleneck: parallel AI lanes produce more code than one person can review well; quality drops or merges stall | Launch-blocking | Commitment ≤ 30 pts; PR < 400 lines; requesting-code-review skill before human review; review GW critical path first daily; track review queue age (< 1 day) | Tech Lead | Every sprint |
| R-02 | Codec disagreement between Go and jPOS appears late (padding, LL counts in bytes vs hex chars, bitmap case) | Launch-blocking | Single `packager-spec.yaml`, generated packagers, golden + negative vectors on both sides from Sprint 1, moov-io cross-check | GW + ISS | End of Sprint 1 |
| R-03 | Reversal / duplicate edge cases create ledger drift that only shows under load | Launch-blocking | Property-based interleaving tests (MCN-402-AC4); chaos suite nightly from Sprint 6; `mcn_ledger_discrepancy` alert | ISS | Sprint 5–6 |
| R-04 | Contract churn: frontend built on mocks diverges from backend reality | Launch-blocking | Contract PR day 1; MSW generated from spec; provider response validation in tests; integration checkpoint per slice | Tech Lead | Each slice |
| R-05 | Concurrency bugs in MUX/SAF (goroutine leaks, double send, lost wake-ups) | Launch-blocking | `-race` always; owned goroutines with errgroup; 1,000-request randomized MUX test; SKIP LOCKED worker tests with 4 workers | GW | Sprint 2, 5 |
| R-06 | 200 TPS not reachable on laptop (Docker overhead, DB locks on hot card) | Fast-follow | Load spike in Sprint 3/4 (assumption A2); lock scope minimal; connection pool tuning; accept 100 TPS target with documented reason if needed | ISS + PLAT | Sprint 4 |
| R-07 | PCI slip: a debug log or fixture prints a PAN or PIN block | Launch-blocking | Masker in logging layer, `pci-scan` in CI from Sprint 0/6, no `msg.dump()`, test helpers only | All | Every PR |
| R-08 | Scope creep from "production realism" (HSM command sets, scheme specifics) | Fast-follow | Out-of-scope list in PRD; new ideas go to v2 backlog; brainstorming only for current story | Tech Lead | Planning |
| R-09 | Flaky E2E/chaos tests erode trust in CI | Fast-follow | No sleeps, deterministic clocks, animations off in E2E, quarantine lane with 48 h fix SLA | WEB + PLAT | From Sprint 4 |

## Paper tigers (overblown)

| Concern | Why it is not a real risk |
| --- | --- |
| "jPOS is too old / hard to learn" | It is the de-facto open-source ISO 8583 stack; participants and Q2 map directly to our design; learning it is a goal |
| "Kafka is overkill for one laptop" | Single-node KRaft is light; it is only used for settlement events where the outbox/inbox patterns are the learning target |
| "Software HSM is not realistic" | Key hierarchy, KCVs, translation and verification flows are the same; hardware specifics are documented as out of scope |
| "Two backend languages slow us down" | Lanes are independent; contracts isolate them; both languages are core skills for target roles |

## Elephants (not discussed enough)

| Concern | Investigation |
| --- | --- |
| E-1 How much of the code the product owner can explain in an interview if agents write most of it | Every plan in `docs/plans/` reviewed and annotated by the Tech Lead; one blog post per epic explaining key code; pair-read one PR per week |
| E-2 Whether Easy mode really works for non-technical people | Hallway tests with 3 people at R3 and R4 (assumption A4); adjust glossary |
| E-3 Maintenance cost of keeping docs, contracts and code in sync | DoD requires doc updates in the same PR; monthly doc review in retro |
| E-4 Motion and realtime updates hurting performance on modest laptops | Performance budget checks in Storybook and Lighthouse on Overview and Journey at R3 |

## Action plans for launch-blocking tigers

| Risk | Action | Owner | Due |
| --- | --- | --- | --- |
| R-01 | Add review-queue metric to board; enforce PR size check in CI | Tech Lead | Sprint 0 |
| R-02 | Generate both packagers from spec; add negative vectors | GW + ISS | Sprint 1 |
| R-03 | Property-based interleaving test + nightly chaos | ISS + PLAT | Sprint 5–6 |
| R-04 | Spectral + oasdiff + provider validation in CI | PLAT | Sprint 0 |
| R-05 | MUX randomized concurrency test and goroutine leak check (goleak) | GW | Sprint 2 |
| R-07 | Masker + scanner in Sprint 0 skeletons, full scanner in MCN-506 | PLAT | Sprint 0 / 6 |
