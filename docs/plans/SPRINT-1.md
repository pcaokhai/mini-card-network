# Sprint 1 — Execution Overview

> **For Claude:** This file orchestrates Sprint 1. Each story has its own plan (`MCN-10x.md`). Execute each plan with superpowers:subagent-driven-development (or superpowers:executing-plans), one worktree per story, following root `CLAUDE.md` §4. Contract-first check: `contracts/openapi.yaml` already has every `/v1/lab/messages/*` path and schema this sprint needs (merged in MCN-003's doc pack) — **no new contract PR is needed before Wave 1 starts.**

**Sprint goal:** Learners can dissect any MCN-87A message. **Duration:** 2 weeks. **Commitment:** 19 points. **Release:** — (S1 ships with R1 once the slice's integration checkpoint passes; no standalone release this sprint per `docs/07-delivery-plan.md` §3).

## Waves

| Wave | Stories (parallel lanes) | Starts when |
| --- | --- | --- |
| 1 | GW **MCN-101** ISO 8583 codec ∥ ISS **MCN-102** jPOS packager ∥ WEB **MCN-104** Message Lab screen | Sprint 0 merged (MCN-101/102 need MCN-003's `contracts/iso8583/`; MCN-104 needs MCN-004's shell + MSW, not MCN-101) |
| 2 | GW **MCN-103** Lab API | MCN-101 merged |

MCN-104 (WEB) does **not** wait for MCN-103 (GW) — it builds against the MSW mocks MCN-004 already generated from `contracts/openapi.yaml`'s `/v1/lab/messages/*` paths, per `docs/07-delivery-plan.md`'s "frontend never waits for backend" rule. The integration checkpoint (real gateway, `NEXT_PUBLIC_API_MOCKS=false`) happens once both MCN-103 and MCN-104 are merged.

MCN-101 and MCN-102 both read `contracts/iso8583/packager-spec.yaml` but write to disjoint directories (`gateway-go/` vs `issuer-jpos/`) — safe to run in parallel worktrees.

## Plans

| Plan | Lane | Depends on |
| --- | --- | --- |
| [MCN-101](MCN-101.md) | GW | MCN-003 (merged) |
| [MCN-102](MCN-102.md) | ISS | MCN-003, MCN-005-ISS (both merged) |
| [MCN-103](MCN-103.md) | GW | MCN-101 |
| [MCN-104](MCN-104.md) | WEB | MCN-004 (merged) |

None of these plans were executed in a sandbox before being written (unlike Sprint 0's, several of which were) — every task has its own failing-test-first verification command; run each one for real and fix with superpowers:systematic-debugging before moving on. Expect at least one version-drift surprise per lane, the same pattern Sprint 0 hit repeatedly (Spring Boot 4's test-metrics gate, Javalin 7's routing API, logstash-logback-encoder 9's masking config, jPOS 3.0.1's env-var syntax) — MCN-102's plan already flags the jPOS `GenericPackager` field-class names as unconfirmed and puts a schema-probe step first for exactly this reason.

## Worktree setup (run on `main`, Sprint 0 already merged)

```bash
mkdir -p ../mcn-worktrees
git worktree add ../mcn-worktrees/gw-101-codec           -b feat/MCN-101-iso8583-codec
git worktree add ../mcn-worktrees/iss-102-packager       -b feat/MCN-102-jpos-packager
git worktree add ../mcn-worktrees/web-104-message-lab    -b feat/MCN-104-message-lab
# after MCN-101 merges
git worktree add ../mcn-worktrees/gw-103-lab-api         -b feat/MCN-103-lab-api
```

## Rulings for the whole sprint

| Ruling | Why | Cost if wrong |
| --- | --- | --- |
| Both codecs (`gateway-go`'s Go codec, `issuer-jpos`'s `iso87ascii.xml`) are generated from the single `contracts/iso8583/packager-spec.yaml` (ADR-003) — never hand-edited independently | Byte-level agreement by construction, not by manual sync | Wire mismatch caught by golden vectors, not production |
| "Easy name" per data element defaults to `packager-spec.yaml`'s technical `name`; only MTI, DE 22, DE 55 and DE 90 get a hand-written learner-friendly override (MCN-103's ruling) | Avoids inventing 40 bespoke labels for a v1 lab screen; extensible later | A few field names read more technical than ideal until refined |
| MCN-101's `FieldSpec` gains a `Name` field (extending Sprint 0's original generator) so MCN-103 can build `technicalName` without re-deriving it from YAML at runtime | Small additive change is cheaper than a second read of `packager-spec.yaml` inside the HTTP layer | None — backward compatible |
| PAN (DE 2) is masked in every Lab API response, including the raw segment text, not just the decoded field value | AC4's literal wording ("PAN is masked in every response") | A learner could otherwise recover a real-looking PAN from the raw hex view |
| MCN-104 builds and ships against MSW mocks; switching to the real MCN-103 gateway is `NEXT_PUBLIC_API_MOCKS=false`, no code change | Delivery plan's parallel-lane rule | None if the OpenAPI contract holds; a shape mismatch would surface at the integration checkpoint |

## Sprint 1 exit checklist

- [ ] `make -C gateway-go test` green: codec, vectors, 60s fuzz (CI), moov-io cross-check, Lab API, OpenAPI response validation
- [ ] `make -C issuer-jpos test` green: `iso87ascii.xml` generated, golden vectors byte-identical, sensitive-DE log masking
- [ ] `make -C web-next lint test` green: Message Lab screen, both locales' `lab.*` keys, Storybook stories build
- [ ] Integration checkpoint: `make up`, `FF_S1_MESSAGE_LAB` on, `NEXT_PUBLIC_API_MOCKS=false`, paste a raw message in the running web console and see it decoded by the real gateway
- [ ] `docs/02` §9 updated if any dependency (moov-io/iso8583, kin-openapi) resolved to an unexpected version
