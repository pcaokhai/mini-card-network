# Mini Card Network — Technical Documentation Pack

Handover package for the development team (human or Claude Code lanes). Everything needed to start Sprint 0 is here; nothing required lives only in chat or in someone's head.

| # | Document | Purpose | Primary audience | Status |
| --- | --- | --- | --- | --- |
| 01 | [PRD](01-prd.md) | What we build and why, scope per release | Everyone | Approved v1.0 |
| 02 | [Software Architecture Document](02-software-architecture.md) | C4 views, runtime flows, cross-cutting concerns, deployment, motion system | Engineers | Approved v1.0 |
| 03 | [ISO 8583 Interface Specification](03-iso8583-interface-spec.md) | Wire-level contract between gateway, switch and issuer | ISS, GW | Approved v1.0 |
| 04 | [API Contract](04-api-contract.md) | REST + WebSocket conventions and endpoint catalogue (normative source: `contracts/`) | WEB, GW, ISS, SET | Approved v1.0 |
| 05 | [Data Model](05-data-model.md) | Schemas, ownership, migration rules; baseline DDL in `assets/baseline-schema.sql` | ISS, GW, SET | Approved v1.0 |
| 06 | [User Stories](06-user-stories.md) | Epics, stories, acceptance criteria, slice pairing, estimates | Everyone | Ready for Sprint 0–3; later sprints refined at planning |
| 07 | [Delivery Plan](07-delivery-plan.md) | Lanes, sprints, parallelization, dependencies, DoR/DoD, release plan, Claude Code playbook | Everyone | Approved v1.0 |
| 08 | [Test Strategy and Scenarios](08-test-strategy.md) | Test pyramid, gates, contract/chaos/perf/security testing, acceptance scenarios | Everyone | Approved v1.0 |
| 09 | [Risk Register (Pre-mortem)](09-risk-register.md) | Tigers, paper tigers, elephants, mitigations | Tech Lead | Living |
| 10 | [Engineering Standards](10-engineering-standards.md) | SOLID, patterns, conventions per language, security, review checklist | Engineers | Approved v1.0 |
| — | [ADRs](adr/) | Architecture decisions with context and consequences | Engineers | Living |
| — | [Plans](plans/) | Implementation plans per story (writing-plans skill output) | Engineers | Created per story |

## Reading order

- **Day 1, everyone:** 01 → 02 (§1–5) → 07 → the stories of your first sprint in 06.
- **ISS / GW engineers:** 03 in full, 05, 10 (§Java or §Go), `issuer-jpos/CLAUDE.md` or `gateway-go/CLAUDE.md`.
- **WEB engineers:** 04, `contracts/openapi.yaml`, 02 §7.9 (motion), 10 (§TypeScript), `web-next/CLAUDE.md`, the design canvas.
- **SET engineers:** 02 §6.5, 05, 04 (§Settlement), `settlement/CLAUDE.md`.
- **QA:** 06, 08, 03 §8 (response codes) and §9 (timers).

## Document conventions

- Language: English. Dates ISO 8601. Money in minor units with ISO 4217 numeric currency.
- IDs: stories `MCN-<epic><nn>` (e.g. `MCN-303`); acceptance criteria `MCN-303-AC2`; test scenarios `TS-<nn>`; decisions `ADR-<nnn>`; risks `R-<nn>`; requirements `FR-<nn>` / `NFR-<nn>`.
- Normative words: MUST, MUST NOT, SHOULD, MAY (RFC 2119).
- Change control: docs change through PRs like code. A behavior, contract or schema change updates the relevant doc in the same PR. Breaking contract changes need an ADR.
- Versioning: each doc carries a version in its header; bump minor for additions, major for breaking changes.
