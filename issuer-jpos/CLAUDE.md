# issuer-jpos — CLAUDE.md

Issuer host. Receives ISO 8583 from the gateway/switch, authorizes against its own ledger, answers, and publishes events through the outbox. Also exposes the Issuer Admin REST API used by the web console.

Lane: **ISS**. Owns: `issuer-jpos/**`, database `issuer`, migrations `issuer-jpos/src/main/resources/db/migration/`.

## Commands

```bash
./gradlew build                 # compile + every test
./gradlew test                  # every test: unit, Testcontainers integration, ISO golden vectors, ArchUnit
./gradlew spotlessApply         # format (google-java-format)
./gradlew spotlessCheck test    # what CI runs
./gradlew run                   # start Q2 locally (needs `make up` infra)
```

## Layout

```
src/main/java/io/mcn/issuer/
  domain/            Pure model: Account, Card, Money, Hold, Journal, ResponseCode, TransactionType. No jPOS, no JDBC.
  application/       Use cases as ports + services: AuthorizePurchase, ReverseTransaction, CompleteHold, Cutover...
    port/in/         Inbound ports (commands/queries)
    port/out/        Outbound ports (AccountRepository, TranLogRepository, OutboxPort, SecurityModule, Clock)
  adapter/
    iso/             jPOS glue: listener, participants, ISOMsg ↔ command mapping, ResponseCodeMapper
    persistence/     JDBC repositories (HikariCP), Flyway
    security/        JCESecurityModule adapter implementing SecurityModule port
    admin/           Issuer Admin REST API (Javalin QBean), DTOs, problem+json mapper
    messaging/       Outbox relay → Kafka
  config/            Typed configuration records, validated at startup
src/dist/deploy/     Q2 descriptors, numbered by start order: 00_logger.xml, 05_datasource.xml,
                     10_security.xml, 20_txnmgr.xml, 30_server.xml, 40_admin_api.xml, 50_outbox_relay.xml
src/dist/cfg/        iso87ascii.xml packager (generated from contracts/iso8583/packager-spec.yaml), app.yaml
```

## jPOS rules

- **Listener only enqueues.** `ISORequestListener` puts the message in the space for the TransactionManager and returns `true`. No DB, no business logic, no blocking I/O on the channel thread.
- **One participant, one responsibility.** Order in `20_txnmgr.xml`: `ParseAndValidate → Deduplicate → CheckCard → VerifySecurity → CheckLimits → Authorize → LogAndOutbox → Respond`. Chain of Responsibility; add behavior by adding a participant, not by growing one.
- Participants are thin adapters: translate the jPOS `Context` into an application command, call the use case, write the result back. Business rules live in `application/` and `domain/`.
- Context keys are constants in `CtxKey` enum. Never use string literals for keys.
- A participant that decides a response code writes `CtxKey.RESULT` and returns `ABORTED | READONLY`; `Respond` is the only participant that sends. Use groups/selectors for MTI routing (`0100`, `0200`, `0220`, `0420`, `0800`, `0500`).
- Always respond, even on unexpected exceptions (RC `96`), except to 0x20/0x21 advices where the rule is: acknowledge with 0x30 once the advice is durably recorded.
- Q2 descriptors wire components; they contain no business constants other than config references (`${env:...}`).

## Transactions and money

- Authorization runs in one DB transaction: lock the account row (`SELECT ... FOR UPDATE`), check available balance and limits, write `tran_log`, `auth_hold` or journal + postings, `velocity_counter`, `outbox_event`, commit, then respond.
- `Money` is a record `(long minor, Currency currency)` with arithmetic that throws on currency mismatch and on overflow (`Math.addExact`).
- Reversal matching uses field 90 (`OriginalData` value object). A reversal that finds nothing is recorded as `REVERSAL_WITHOUT_ORIGINAL` so a late original is declined with RC `94`/`05` per ISO spec §7.
- Dedupe: unique constraint `uq_tran_dedupe` is the source of truth. On conflict, load the stored response and replay it byte-for-byte (same RC, same field 38).

## Java rules

- Java 25: records for value objects and DTOs, sealed interfaces for results (`AuthorizationResult permits Approved, Declined`), pattern matching `switch` over sealed types, `var` only when the type is obvious.
- Constructor injection only, all fields `final`. No static mutable state. No service locators.
- No `null` returns from public methods: return `Optional` for "maybe" queries; never use `Optional` as a field or parameter.
- Exceptions: domain exceptions extend `IssuerException` and carry a `ResponseCode`. `ResponseCodeMapper` is the single mapping to field 39 and to HTTP problem types.
- Logging: SLF4J with JSON encoder; use `IsoLogFormatter` for messages (masks 2, 35, 45, 52, 55 and truncates 64/128). Never `msg.dump()` to logs.
- Lombok is not used. Keep classes small; prefer composition.

## Tests

- JUnit 5 + AssertJ. One test class per production class; name tests `should_<behavior>_when_<condition>`.
- Domain and application tests have no Spring/jPOS/DB.
- Integration tests: Testcontainers Postgres + real Flyway migrations; a `TestIssuer` harness that runs the TransactionManager in-process and sends ISOMsg directly.
- ISO golden vectors in `contracts/iso8583/vectors/` must pack/unpack byte-identical with the packager.
- ArchUnit rules: `domain` depends on nothing; `application` depends only on `domain`; adapters never depend on each other.
- Concurrency test for MCN-302: 200 parallel purchases on one card never drive `available_balance` below the floor.
