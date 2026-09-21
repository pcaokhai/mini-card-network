# settlement — CLAUDE.md

Consumes transaction events from both hosts, builds clearing records, reconciles acquirer vs issuer per business date, manages breaks, computes net positions and writes the clearing file. Exposes the Settlement REST API.

Lane: **SET**. Owns: `settlement/**`, database `settlement`, migrations `settlement/src/main/resources/db/migration/`.

## Commands

```bash
./gradlew build
./gradlew test integrationTest     # Testcontainers Postgres + Kafka
./gradlew spotlessApply archTest
./gradlew bootRun
```

## Layout (hexagonal)

```
io.mcn.settlement
  domain/        ClearingRecord, ReconRun, Break, BreakType, NetPosition, BusinessDate, Money — pure
  application/   ReconcileBusinessDay, ResolveBreak, GenerateClearingFile, IngestEvent (ports in/out)
  adapter/
    kafka/       Consumers (manual ack after commit), event DTOs generated from contracts/events/
    persistence/ Spring Data JDBC repositories, Flyway
    web/         REST controllers, problem+json advice, OpenAPI-validated DTOs
    file/        Clearing file writer (CSV + trailer + SHA-256)
  config/
```

## Rules

- **Idempotent consumer (Inbox pattern):** insert `inbox_event(event_id)` and apply the event in the same DB transaction; on duplicate key, skip and ack. Ack the Kafka offset only after commit.
- Events are keyed by `card_id`/`rrn` so per-transaction order is preserved; still design every handler to be order-tolerant (a reversal may arrive before its original).
- Reconciliation is a **pure function** `reconcile(acquirerRecords, issuerRecords) -> ReconResult` in `domain`; I/O happens around it. It must be deterministic and fully unit-tested with table-driven cases for every `BreakType`.
- The clearing file is deterministic: same inputs ⇒ byte-identical file and checksum. Sort records by (RRN, STAN).
- Clearing file generation is refused while breaks are `OPEN` (HTTP 409 problem `open-breaks`).
- Spring: constructor injection, `@Transactional` only on application services, no field injection, no business logic in controllers.
- Same Java, logging, money and test rules as `issuer-jpos/CLAUDE.md` (sections "Java rules" and "Tests").
