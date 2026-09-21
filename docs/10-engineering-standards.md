# Engineering Standards

Version 1.0 · 2026-09-21 · Owner: Tech Lead · Applies to all code, tests, scripts and configuration in this repository

These are the rules an experienced engineer applies on a production payment system. Reviewers (human or the requesting-code-review skill) check against this document. "MUST" items block merge.

## 1. Principles

| Principle | What it means here | Example |
| --- | --- | --- |
| Single Responsibility | A class/function/package has one reason to change | `Deduplicate` participant only checks duplicates; it does not authorize |
| Open/Closed | Extend by adding, not by editing | New velocity rule = new `VelocityRule` strategy registered in config |
| Liskov Substitution | Implementations honor the port's contract, including errors and timing | Every `SecurityModule` implementation passes the same contract test suite |
| Interface Segregation | Small, consumer-owned interfaces | Go `txn` needs `Sender` (one method), not the whole `isonet.Conn` |
| Dependency Inversion | Domain/application depend on ports; adapters implement them | `AuthorizePurchase` depends on `AccountRepository`, not JDBC |
| KISS / YAGNI | Build what the story needs; no speculative generality | No plugin system until the third rule type exists |
| DRY with rule of three | Duplicate once, abstract on the third occurrence, when the concept is the same | Two similar DTOs in different bounded contexts stay separate |
| Fail fast | Validate config and inputs at the edge; crash on invalid startup state | Missing LMK ⇒ service does not start |
| Explicit over implicit | No magic: no reflection-based wiring beyond the framework, no global state | Constructor injection, typed config |
| Make illegal states unrepresentable | Types encode invariants | `Money` cannot mix currencies; sealed `AuthorizationResult`; Go state machine rejects illegal transitions |

## 2. Architecture rules

- **Hexagonal layering** in every backend service: `domain` → `application` (ports + use cases) → `adapter` (inbound: ISO, REST, Kafka; outbound: DB, HSM, Kafka, HTTP). Dependencies point inward only. MUST be enforced by ArchUnit (Java) and a Go import-boundary test / `depguard`.
- Domain code MUST NOT import frameworks (jPOS, Spring, Javalin, chi, pgx, Kafka clients) or perform I/O.
- One use case = one application service method with a command/query object; transaction boundaries live in application services.
- Cross-service communication only via ISO 8583, REST contracts or Kafka events. No shared databases, no shared libraries with business logic (shared code is limited to generated contract code).
- Every external dependency behind a port with a test double (fake preferred over mocks).

## 3. Design patterns (use for the named problem)

| Pattern | Problem it solves | Where |
| --- | --- | --- |
| Chain of Responsibility | Ordered, independent authorization checks | jPOS participants |
| Strategy | Interchangeable algorithms chosen by config/data | Field codecs by type, routing by BIN, velocity rules, STIP rules, packager |
| State | Legal lifecycle transitions | Gateway `txn` state machine, SAF item, settlement day stage |
| Builder | Complex message construction with validation | ISO message builders (`Purchase0200Builder`) |
| Factory | Creating the right handler per MTI class | Participant group selector, `IsoHandlerFactory` |
| Adapter / Port | Isolating frameworks and external systems | HSM, DB, Kafka, Toxiproxy, HTTP |
| Repository + Unit of Work | Persistence with transactional boundaries | Issuer JDBC repos, Go `store.Tx` |
| Transactional Outbox / Inbox | Reliable event publishing / idempotent consumption | Issuer, gateway → Kafka → settlement |
| Store-and-Forward + Competing Consumers | Guaranteed delivery of advices | SAF worker with `SKIP LOCKED` |
| Correlation Identifier | Match async responses | MUX (STAN + DE 7) |
| Idempotency Key | Safe retries | REST writes, ISO dedupe |
| Circuit Breaker + Fallback | Protect callers, degrade gracefully | Switch STIP |
| Retry with exponential backoff + jitter | Transient failures | Reconnect, SAF |
| Observer / Pub-Sub | Fan-out of state changes | WS hub, event bus in web |
| Value Object | Invariants on small values | `Money`, `Stan`, `Rrn`, `MaskedPan`, `BusinessDate`, `Kcv` |
| Specification | Composable business predicates | Card eligibility checks |

Anti-patterns that fail review: god services, anemic "manager" classes holding all logic, boolean flag parameters that switch behavior, `util`/`helpers` dumping grounds, deep inheritance, catching and ignoring errors, stringly-typed IDs and statuses, singletons with mutable state, temporal coupling without enforcement.

## 4. Code conventions

### 4.1 All languages

- Names from the domain glossary (root `CLAUDE.md` §8). Functions are verbs (`authorizePurchase`), booleans are predicates (`isExpired`), collections are plural.
- Functions ≤ ~30 lines, ≤ 4 parameters (use a parameter object beyond that), cyclomatic complexity ≤ 10.
- Files ≤ ~300 lines; split by responsibility, not by size alone.
- No magic numbers or strings: constants with meaning (`ISO_REQUEST_TIMEOUT`, `ResponseCode.INSUFFICIENT_FUNDS`).
- Comments explain *why* (decisions, spec references like "ISO spec §7.3"), never restate code. TODOs need an issue id.
- Public APIs (exported Go identifiers, public Java types, shared TS components) have doc comments.
- Formatting is automatic (Spotless/google-java-format, gofmt/goimports, Prettier); never argue style in review.

### 4.2 Java 25 (issuer-jpos, settlement)

- Records for value objects/DTOs/commands; sealed interfaces + pattern-matching `switch` for closed result sets.
- `final` fields, constructor injection, immutable collections (`List.copyOf`). No Lombok.
- No `null` from public methods; `Optional` only as return type. Validate arguments with `Objects.requireNonNull` at boundaries.
- Money arithmetic via `Money` with `Math.addExact/subtractExact`. No `double`, no `BigDecimal` for amounts.
- Checked exceptions only at adapter edges; domain uses unchecked typed exceptions carrying a `ResponseCode` or problem type.
- Streams for transformations, loops for side effects; no nested streams beyond two levels.
- Virtual threads allowed for blocking I/O in adapters (Javalin, relays); never pin with `synchronized` around I/O.
- Package-private by default; `public` only for ports and API.

### 4.3 Go (gateway-go, switch)

- Effective Go + Go Code Review Comments. `gofmt`, `goimports`, `golangci-lint` (errcheck, govet, staticcheck, revive, gocyclo, depguard, gosec, bodyclose, contextcheck).
- `ctx context.Context` first parameter for anything that blocks; never store contexts in structs.
- Errors: `%w` wrapping with context; sentinel or typed errors; `errors.Is/As`; never compare error strings; no `panic` in library code.
- Concurrency: every goroutine owned and cancellable; channels have one closer (the sender); protect shared state with a mutex or confine it to one goroutine; `goleak` in tests of long-running components.
- Constructors `NewX(deps) (*X, error)` validate dependencies; zero values are either useful or impossible.
- Interfaces small and at the consumer; accept interfaces, return structs.
- Package names short, singular, no `util`/`common`.

### 4.4 TypeScript / React (web-next)

- `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`; no `any`; `unknown` + Zod at trust boundaries.
- Server Components by default; client components small and leaf-level; no `useEffect` for data fetching (TanStack Query).
- Components are pure functions of props; side effects in hooks named `useX`; one component per file; named exports.
- Derived state is computed, not stored. Keys are stable ids, never indexes for dynamic lists.
- Styling with Tailwind tokens; no inline magic colors; motion only via motion tokens.
- Accessibility is part of done: semantic elements, labels, focus management, live regions for async results.

### 4.5 SQL

- Explicit column lists; no `SELECT *` in application code.
- Parameterized queries only (sqlc, JDBC prepared statements); never string concatenation.
- Lock intentionally (`FOR UPDATE`, `SKIP LOCKED`) and document why in a comment with the story id.
- Every query used on a hot path has an index listed in `docs/05` §6; EXPLAIN checked in the PR for new hot queries.

## 5. Error handling

- Classify errors: **validation** (client fault), **business decline** (expected outcome, e.g. RC 51), **transient** (retryable), **fatal** (bug/config).
- Business declines are values, not exceptions, in the domain (`Declined(ResponseCode)`).
- One mapper per service converts errors to field 39 and to problem+json. Never leak stack traces or SQL to clients.
- Retry only transient errors, with backoff + jitter and a cap; never retry non-idempotent operations without an idempotency key.
- Every caught exception is either handled (with a log explaining the decision) or wrapped and rethrown.

## 6. Observability

- Log levels: ERROR = needs human action; WARN = degraded but handled (reversal, retry); INFO = business events (one line per transaction outcome); DEBUG = diagnostics (off by default).
- Structured JSON only, standard fields (`docs/02` §7.6); messages are constant strings, data goes in fields.
- Never log card data unmasked, secrets, keys, full ISO dumps, or request bodies with PIN blocks.
- Metrics: RED (rate, errors, duration) for every inbound interface; USE for pools/queues; business metrics listed in `docs/02` §7.6. Metric names `mcn_<noun>_<unit>`; label cardinality bounded (no RRN/PAN labels).
- Tracing: span per inbound request, DB transaction, outbound call, ISO exchange (`iso.mti`, `iso.stan`, `iso.rrn`, `iso.rc` attributes).

## 7. Security

- PCI rules in root `CLAUDE.md` §6.2 are absolute. Masking utilities are the only way to render card data.
- Secrets only from env/secret files; `.env` git-ignored; gitleaks in CI.
- Validate all input at the edge (OpenAPI validation, ISO field validation); reject unknown fields in admin APIs.
- Principle of least privilege: each service DB role only has grants on its own schema; audit tables have no UPDATE/DELETE grants.
- Dependencies: pinned versions, Renovate/Dependabot weekly, no critical CVEs; licenses must be permissive (MIT, Apache-2.0, BSD, EPL for jPOS-related is acceptable with note).
- Constant-time comparison for MACs and KCVs.

## 8. Concurrency and consistency

- One DB transaction per business operation; no network calls inside DB transactions except the ISO send/receive rule documented in `docs/02` §6.1 (the gateway commits `SENT` before sending).
- Pessimistic lock for balance mutation; optimistic version for admin edits; both covered by concurrent tests.
- Idempotency at every boundary (`docs/02` §7.2).
- Timeouts on every I/O (DB, HTTP, ISO, Kafka); no unbounded waits.
- Graceful shutdown: stop intake → finish in-flight within the drain window → flush outbox/SAF state → close resources.

## 9. Performance

- Measure before optimizing; performance changes need a benchmark or load result in the PR.
- No N+1 queries; batch where natural (outbox relay, SAF claim).
- Bounded queues and pools with metrics; backpressure instead of unbounded buffering.
- Web: route-level code splitting, virtualized long lists, batched realtime updates, Lighthouse performance ≥ 90 on Overview.

## 10. Testing

See `08-test-strategy.md`. Additionally: arrange-act-assert structure; one behavior per test; test doubles are fakes by default (in-memory repositories, fake issuer), mocks only for interaction-critical ports; no test depends on another test's state.

## 11. Git and pull requests

- Conventional Commits (`feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `build`, `ci`) with scope = lane (`iss`, `gw`, `web`, `set`, `plat`, `contracts`) and story id.
- One story per PR; ≤ 400 changed lines excluding generated files; draft PR early for large stories.
- PR description: what/why, AC mapping to tests, screenshots or clip for UI, risk and rollback note, docs updated.
- Never force-push to `main`; no merge commits on `main`.

## 12. Code review checklist

- [ ] Story AC each proven by a named test; tests fail without the change
- [ ] Layering respected (no framework in domain, adapters don't call each other)
- [ ] Patterns used as intended; no new abstraction without a second use
- [ ] Money, PCI, idempotency, reversal and ledger rules honored
- [ ] Errors classified and mapped in the single mapper; no swallowed errors
- [ ] Concurrency: ownership, cancellation, locking, timeouts
- [ ] Logs/metrics/traces added; no sensitive data; bounded label cardinality
- [ ] Contracts respected; generated code untouched; migrations forward-only and numbered
- [ ] Names, function size, comments explain why
- [ ] Docs updated in the same PR when behavior/contract/schema changed

## 13. Documentation

- ADR (`docs/adr/ADR-<nnn>-<slug>.md`, template in `ADR-000`) for any decision that is hard to reverse, crosses lanes, or changes a contract.
- Each service README: purpose, how to run, config table, ports, troubleshooting.
- Runbooks for every alert (`docs/runbooks/`).
- Plans in `docs/plans/` are kept after merge as a learning record.
