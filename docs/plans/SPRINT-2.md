# Sprint 2 — Execution Overview

> **For Claude:** This file orchestrates Sprint 2. Each story has its own plan (`MCN-20x.md`). Execute each plan with superpowers:subagent-driven-development (or superpowers:executing-plans), one worktree per story, following root `CLAUDE.md` §4. Contract-first check: `contracts/openapi.yaml` and `contracts/ws-events.schema.json` already have every `/v1/links`, `/v1/network/events` and `link.status`/`network.event` shape this sprint needs (from the original doc pack) — **no new contract PR is needed before Wave 1 starts.**

**Sprint goal:** The acquirer link to the issuer connects, signs on, echoes, survives a drop, and the ops console shows it live. **Duration:** 2 weeks. **Commitment:** 8 (MCN-201) + 8 (MCN-202) + 5 (MCN-203) + 5 (MCN-204) + 5 (MCN-205) = 31 points. **Release:** `docs/releases/R2.md`, written after the integration checkpoint passes.

## Waves

| Wave | Stories (parallel lanes) | Starts when |
| --- | --- | --- |
| 1 | ISS **MCN-201** issuer ISO server + network mgmt ∥ GW **MCN-202** connection manager ∥ WEB **MCN-205** Network Operations screen | Sprint 1 merged. MCN-201 and MCN-202 are independent (different services, both consume `contracts/iso8583/packager-spec.yaml` which is unchanged this sprint). MCN-205 builds against MSW mocks (own fixture, `contracts/fixtures/network.json`) and does not wait for MCN-202 or MCN-204. |
| 2 | GW **MCN-203** MUX | MCN-202 merged (extends `internal/isonet`, refactors `Supervisor`'s STAN ownership) |
| 3 | GW **MCN-204** Network API + WS events | MCN-203 merged (manual echo/sign-on/sign-off route through the MUX) |

MCN-203 and MCN-204 are sequential within the GW lane — both modify `internal/isonet/supervisor.go` and MCN-204's manual triggers assume the MUX-based `Send` path MCN-203 introduces. Running them in parallel worktrees would produce a merge conflict on the same file with logically dependent changes, so they run one after another, matching the dependency already noted in `docs/07-delivery-plan.md`'s wave notation for this slice.

MCN-201 (ISS) has no downstream dependency within this sprint's WEB/GW stories — the acquirer-side `Supervisor` (MCN-202) talks to MCN-201's server over raw sockets per the shared `contracts/iso8583/packager-spec.yaml`, not by importing anything from `issuer-jpos/`. Both can be built and merged independently; the integration checkpoint (below) is where they first talk to each other for real.

## Plans

| Plan | Lane | Depends on |
| --- | --- | --- |
| [MCN-201](MCN-201.md) | ISS | Sprint 1 (merged) |
| [MCN-202](MCN-202.md) | GW | Sprint 1 (merged) |
| [MCN-203](MCN-203.md) | GW | MCN-202 |
| [MCN-204](MCN-204.md) | GW | MCN-203 |
| [MCN-205](MCN-205.md) | WEB | Sprint 1 / MCN-004 (merged) |

None of these plans were executed in a sandbox before being written. Expect at least one library/framework surprise per lane, consistent with every prior sprint (Spring Boot 4, Javalin 7, logstash-logback-encoder 9, jPOS 3.0.1, moov-io/iso8583, golangci-lint v2 in Sprint 0/1). MCN-202's Task 3 explicitly flags the `goose`+`pgx/stdlib` wiring as unconfirmed and puts a "read the real package docs" check before the placeholder code; MCN-204's Task 0 requires reading the real WS hub API before assuming its method shapes. Fix by reading the actual resolved library/source, per the established pattern — never guess further.

## Worktree setup (run on `main`, Sprint 1 already merged)

```bash
mkdir -p ../mcn-worktrees
git worktree add ../mcn-worktrees/iss-201-network         -b feat/MCN-201-issuer-iso-server
git worktree add ../mcn-worktrees/gw-202-connection-manager -b feat/MCN-202-connection-manager
git worktree add ../mcn-worktrees/web-205-network-ops     -b feat/MCN-205-network-ops
# after MCN-202 merges
git worktree add ../mcn-worktrees/gw-203-mux              -b feat/MCN-203-mux
# after MCN-203 merges
git worktree add ../mcn-worktrees/gw-204-network-api      -b feat/MCN-204-network-api
```

## Rulings for the whole sprint

| Ruling | Why | Cost if wrong |
| --- | --- | --- |
| `acquirer_link` (ISS), `link_state`/`network_event` (GW) get hand-authored DDL this sprint since neither table has an explicit schema anywhere in `docs/05-data-model.md` — only table-name mentions | Docs give conventions (snake_case, TEXT+CHECK status enums, TIMESTAMPTZ, created_at/updated_at) but no concrete columns for these three tables; blocking on a brainstorming session for a schema the conventions already determine would be process theater | A future story could need a column not anticipated here; cheap to add via a new migration, not a redesign |
| No sqlc in `gateway-go` yet — MCN-202's `internal/store` uses hand-written `pgx` queries | Query surface (4 statements) doesn't justify sqlc's generation machinery yet; `gateway-go/CLAUDE.md` names sqlc as the target tool, this is a deferred-not-abandoned decision | If query surface grows fast, a later story pays a one-time sqlc-adoption cost instead of paying it now for nothing |
| STAN issuance moves from `Supervisor` (MCN-202) to `Mux` (MCN-203) once both exist on the same connection | One counter per connection avoids STAN collisions between control (echo/sign-on) and future financial traffic | If skipped, a later financial-message story would hit a STAN collision that's harder to diagnose after the fact |
| RC 91 (transaction refused, link not signed on) is enforced unconditionally starting MCN-201, no feature flag | AC wording in `docs/06-user-stories.md` MCN-201 states it as a hard rule for this story, and gating it would require a config surface nothing else in Sprint 2 needs | None if correct; a flag would be unused complexity |
| MCN-205 ships behind the same MSW-first pattern as MCN-104 (Sprint 1) — real backend is `NEXT_PUBLIC_API_MOCKS=false`, no code change | Consistent parallel-lane rule already proven in Sprint 1 | None if the OpenAPI/WS contracts hold; a shape mismatch surfaces at the integration checkpoint, same as any other slice |

## Sprint 2 exit checklist

- [ ] `make -C issuer-jpos test` green: `AcquirerLinkRepositoryTest` (Testcontainers), `NetworkManagementIntegrationTest` (raw socket sign-on/echo/sign-off/cutover), RC 91 enforcement test
- [ ] `make -C gateway-go test` green: framer, backoff, `LinkRepository` (Testcontainers), `Supervisor` (fake-issuer sign-on/echo/DOWN-detection), `Mux` (correlation, timeout, late-response, 1000-concurrent `-race` on CI), network API handlers, WS broadcast
- [ ] `make -C gateway-go test` Toxiproxy recovery test passes for real with `make up` running (MCN-202 AC4)
- [ ] `make -C web-next lint test build` green: `LinkStatusPill`/`LinksTable`/`EventTimeline`/`TriggerButtons`/`NetworkScreen`, both locales' `network.*` keys, Storybook build, axe check clean
- [ ] Integration checkpoint: `make up`, real issuer + real gateway (`NEXT_PUBLIC_API_MOCKS=false`) — open `/network` in the console, see the real link sign on within a few seconds, click "Echo now" and see the timeline update live, kill the issuer container and see the pill go DOWN then recover on restart
- [ ] `docs/02` §9 updated if any dependency (goose, pgx, testcontainers-go) resolved to an unexpected version
- [ ] `docs/releases/R2.md` written once the checkpoint passes
