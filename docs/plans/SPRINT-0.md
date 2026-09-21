# Sprint 0 — Execution Overview

> **For Claude:** This file orchestrates Sprint 0. Each story has its own plan (`MCN-00x.md`). Execute each plan with superpowers:subagent-driven-development (or superpowers:executing-plans), one worktree per story, following root `CLAUDE.md` §4.

**Sprint goal:** Everyone can build, run and see traces. **Duration:** 1 week. **Commitment:** 23 points. **Release:** R0.

## Waves

| Wave | Stories (parallel lanes) | Starts when |
| --- | --- | --- |
| 1 | PLAT **MCN-001** Monorepo scaffold and CI | Day 1 |
| 2 | PLAT **MCN-002** Local infrastructure ∥ PLAT **MCN-003** Contracts v0 ∥ WEB **MCN-004** Web shell | MCN-001 merged |
| 3 | GW / ISS / SET **MCN-005** Service skeletons (three parallel sub-plans: 005-GW, 005-ISS, 005-SET) | MCN-002 merged |

MCN-002 and MCN-003 are both PLAT: run them in two worktrees; their file sets do not overlap (`infra/` vs `contracts/` + `.spectral.yaml`). The only shared file is the root `Makefile`: MCN-001 creates every target with delegation, so wave 2/3 stories never edit it.

## Plans and verification status

| Plan | Lane | Status in the planning sandbox |
| --- | --- | --- |
| [MCN-001](MCN-001.md) | PLAT | Makefile and repository tests executed and passing |
| [MCN-002](MCN-002.md) | PLAT | Not executable there (no Docker); smoke test provided |
| [MCN-003](MCN-003.md) | PLAT | All code executed: vectors, codec tests, schemas, Spectral lint and ruleset self-test pass |
| [MCN-004](MCN-004.md) | WEB | Scaffolded with Next.js 16.3: 8 tests, tsc, eslint, next build, storybook build pass; browser story tests to run locally |
| [MCN-005-GW](MCN-005-GW.md) | GW | Not executable there (no Go toolchain); every task has a verification command |
| [MCN-005-ISS](MCN-005-ISS.md) | ISS | Not executable there (no Maven Central); version-sensitive APIs marked "check" |
| [MCN-005-SET](MCN-005-SET.md) | SET | Not executable there (no Spring Initializr access) |

## Worktree setup (run on `main` after MCN-001 merges)

```bash
mkdir -p ../mcn-worktrees
git worktree add ../mcn-worktrees/plat-002 -b feat/MCN-002-local-infra
git worktree add ../mcn-worktrees/plat-003 -b feat/MCN-003-contracts-v0
git worktree add ../mcn-worktrees/web-004  -b feat/MCN-004-web-shell
# after MCN-002 merges
git worktree add ../mcn-worktrees/gw-005   -b feat/MCN-005-gw-skeleton
git worktree add ../mcn-worktrees/iss-005  -b feat/MCN-005-iss-skeleton
git worktree add ../mcn-worktrees/set-005  -b feat/MCN-005-set-skeleton
```

## Rulings for the whole sprint

| Ruling | Why | Cost if wrong |
| --- | --- | --- |
| Library versions are resolved at execution time (Go `@latest`, pnpm latest, Maven Central query script, Spring Initializr) and recorded in `docs/02` §9 in the same PR | The plan must not invent version numbers | One follow-up PR to bump |
| Docker image tags below are pinned to known releases; bump to the newest patch when executing and record them | Reproducible infra | Minor |
| MCN-002-AC4 (seed) ships as the `make seed` entry point delegating to per-service seed commands; actual seed data loads with the schemas in MCN-301 / MCN-303 | Schemas do not exist in Sprint 0 | None; AC re-verified in Sprint 3 |
| Java services use the OpenTelemetry Java agent (no tracing code); Go uses the SDK | Least code for AC3 | Agent config drift; covered by smoke test |
| Each Java service carries its own small PAN masker (no shared library) | ADR/standards: no shared business libraries | ~40 duplicated lines |
| Metrics ports follow `docs/02` §8 (9464 GW, 9465 ISS, 9466 SET) | Consistency with SAD | None |

## Sprint 0 exit checklist

- [ ] `make up && make test && make lint && make contracts` green on `main`
- [ ] Web shell reachable at http://localhost:3000 in mock mode and in Storybook
- [ ] `curl localhost:8080/health/ready`, `localhost:8081/health/ready`, `localhost:9466/health/ready` → 200
- [ ] One BFF → service request produces a trace visible in Grafana (Tempo)
- [ ] `docs/02` §9 lists every resolved version
