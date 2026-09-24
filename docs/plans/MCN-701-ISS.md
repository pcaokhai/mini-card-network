# MCN-701-ISS Outbox relay (issuer) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/iss-701`, branch `feat/MCN-701-iss-outbox-relay`. Requires MCN-302, MCN-303 merged (this plan reads `issuer.outbox_event`, already populated by `LogAndOutbox` and, once merged, `MCN-601`'s `CompleteHold`/`HoldExpiryJob`). Runs in parallel with **MCN-701-GW** (disjoint directories, disjoint DB schemas — issuer never reads `acquirer.outbox_event`). One story (3 pts) split across two lanes; see `MCN-701-GW.md` for the gateway half and this plan's Ruling 1 for the shared Kafka topic/partitioning convention, fixed once here since the issuer's `outbox_event` table already exists (the gateway's does not) — this plan's Ruling is authored first and `MCN-701-GW.md` restates it verbatim, mirroring how `MCN-504-GW.md` fixed the rotation vocabulary first because its table existed first.

**Goal:** A relay publishes unpublished `outbox_event` rows, in order, to Kafka topic `issuer.transactions.v1` with `key=RRN` and headers `traceparent`/`event-id`/`event-type`, marking each row published only after the broker ack; `mcn_outbox_lag_seconds` stays under 2s at 200 TPS.

**Architecture:** A new `adapter/kafka/` package (issuer-jpos has no Kafka adapter yet — confirmed by `find issuer-jpos -iname "*Kafka*"` returning nothing) holds `OutboxRelay`, a polling loop (`ScheduledExecutorService`, the same "no existing message-queue framework, poll on an interval" shape `HoldExpiryJob` (`MCN-601.md`) establishes for scheduled work, not duplicated invention) that: reads a batch of unpublished rows ordered by `id` (`AuthHoldRepository`'s sibling for outbox reads — `OutboxEventRepository.findUnpublished(limit)`, using the existing `ix_outbox_unpublished` partial index), maps each row's `payload` JSONB (already shaped close to `contracts/events/transaction-event.schema.json` — `LogAndOutbox`'s existing outbox-write call is the payload's actual producer; this plan does not change what's written, only what reads and republishes it) to a `ProducerRecord<String, String>` (key = the row's `rrn` field from the payload, value = the payload JSON re-serialized to exactly the schema's shape, `KafkaProducer.send` synchronous-enough to await the ack per Global Constraints), sets three headers (`traceparent` — propagated from the payload's own `trace_id` if present, else a fresh W3C-shaped placeholder per root `CLAUDE.md` §6 rule 10's "ISO 8583 carries no trace header: both hosts log the trace id... never add private fields for tracing" — Kafka is not ISO 8583, so this plan **does** attach a real header here, the ISO-specific exemption doesn't apply), `event-id` (the row's own UUID `id`), `event-type` (the row's `event_type` column). On ack, `OutboxEventRepository.markPublished(id)` (one `UPDATE ... SET published_at = now() WHERE id = ?`, idempotent against a relay restart re-sending an already-acked-but-not-yet-marked row — Ruling 2 covers the small at-least-once window this leaves).

**Tech Stack:** Java 25, jPOS Q2 (for the relay's own lifecycle, mirroring `HoldExpiryJob`'s `QBean` wiring), Apache Kafka client (`org.apache.kafka:kafka-clients`, a new dependency — noted per root `CLAUDE.md` §9 "do not add a dependency without noting it in the plan" — this is the first Kafka producer in `issuer-jpos`; `settlement/`'s `adapter/kafka/package-info.java` already anticipates a consumer-side Kafka dependency there, confirming Kafka is an accepted, already-planned-for dependency for this project, not a novel addition), JUnit 5, AssertJ, Testcontainers (Kafka) for the integration test.

**Spec:** `docs/06-user-stories.md` MCN-701-AC1/AC2, `docs/05-data-model.md` §"issuer" row for `outbox_event`/`audit_log` ("Events to Kafka; append-only audit"), `contracts/events/transaction-event.schema.json` (the exact event shape this relay must publish — `eventId`/`eventType`/`source`/`occurredAt`/`businessDate`/`rrn`/`stan`/`transmissionDateTime`/`acquirerId`/`terminalId`/`transactionType`/`amount`/`currency`/`status`/`responseCode`/`authCode`/`originalRrn`/`maskedPan`, `additionalProperties: false`), root `CLAUDE.md` §6 rule 10 (trace propagation), rule 2 (PCI — `maskedPan` only, never full PAN, in the event payload).

## Global Constraints

- The relay never re-orders rows: `findUnpublished` is always `ORDER BY id ASC`, and the relay processes and marks-published sequentially within a batch (no parallel-send-then-batch-ack) — AC1's "publishes... in order" is a hard ordering guarantee, not a best-effort one.
- A row is marked published **only after** the Kafka ack, never before or speculatively — an unacked send on relay crash leaves the row unpublished, safely retried on restart (Ruling 2 covers the resulting at-least-once, never-lost semantics).
- No full PAN or PIN data ever enters an event payload — `payload`'s `maskedPan` field is exactly what `LogAndOutbox` already wrote (masked at write time, unchanged by this plan); the relay is a pure republish, it does not re-derive or unmask anything.
- `event-id` header is the row's own UUID, never regenerated — a Kafka consumer's dedup-by-`event-id` (MCN-703's inbox pattern) only works if the same logical event always carries the same id across republishes/retries.

## Ruling 1: Kafka topic and partitioning convention — fixed here, `MCN-701-GW.md` and `MCN-703.md` must match exactly

Topic names are exactly what `contracts/events/transaction-event.schema.json`'s own `title` already states: `issuer.transactions.v1` (this plan) and `acquirer.transactions.v1` (`MCN-701-GW.md`) — two topics, one per source, not one shared topic with a `source` field used as a filter (the schema's `source` enum `ISSUER`/`ACQUIRER` field exists for a *consumer* to know provenance after the fact, e.g. in `MCN-703`'s `clearing_record.source` column, not as a routing key). **Partitioning key is the transaction's RRN** (`ProducerRecord`'s key parameter, this plan's own choice, restated verbatim in `MCN-701-GW.md`) — not STAN (STAN is only unique per-connection, per `docs/03` §5, so two different terminals' STANs can collide; RRN is globally unique per `docs/03`'s RRN generation rule) and not `eventId` (a random key would scatter a single transaction's `HoldCreated`/`HoldCompleted`/`HoldExpired` sequence across partitions, breaking MCN-703's inbox-pattern per-RRN ordering guarantee it needs from Kafka's own per-partition ordering). Default partition count (`num.partitions` from `infra/docker-compose.yml`'s Kafka service config, unchanged by this plan) is accepted as-is — no custom partitioner class, since RRN-as-key with Kafka's default hash partitioner already gives per-RRN ordering without one.

## Ruling 2: at-least-once delivery is accepted, not eliminated — MCN-703's inbox pattern is the real de-dup boundary

A relay crash between a Kafka ack and this row's `markPublished` write leaves that one row `published_at IS NULL` even though Kafka already has it; on restart, the relay republishes it — a duplicate on the topic. This plan does **not** add a two-phase-commit or transactional-outbox-to-Kafka bridge (Debezium/Kafka Connect CDC) to close that gap — `docs/07-delivery-plan.md`'s Sprint 9 scope for this story is "publishes... marks published after ack" (AC1's literal wording), and `MCN-703.md`'s inbox pattern (dedup on `event-id`, per its own AC1) is the story explicitly designed to absorb this exact at-least-once gap at the consumer. Building exactly-once delivery here would duplicate work `MCN-703` already does and adds infrastructure (a CDC connector) this project has no other use for — YAGNI. This Ruling is restated in `MCN-701-GW.md` since the gateway relay has the identical crash window and the identical answer.

## File map

| Action | Path (under `issuer-jpos/`) |
| --- | --- |
| Modify | `build.gradle` (add `org.apache.kafka:kafka-clients`) |
| Create | `src/main/java/io/mcn/issuer/adapter/persistence/OutboxEventRepository.java`, `OutboxEventRow.java`, test |
| Create | `src/main/java/io/mcn/issuer/adapter/kafka/OutboxRelay.java`, `KafkaProducerFactory.java`, test |
| Create | `src/dist/deploy/80_outbox_relay.xml` |
| Create | `src/test/java/io/mcn/issuer/adapter/kafka/OutboxRelayIntegrationTest.java` (Testcontainers Kafka) |

---

### Task 1: `OutboxEventRepository` (AC1)

**Files:** `adapter/persistence/OutboxEventRepository.java`, `OutboxEventRow.java`, test

**Interfaces:** `OutboxEventRow(UUID id, String aggregateType, String aggregateId, String eventType, String payloadJson, Instant createdAt, Instant publishedAt, int attempts)`. `OutboxEventRepository(DataSource dataSource)`; `.findUnpublished(int limit) -> List<OutboxEventRow>` (ordered by `created_at ASC, id ASC` — `id` is a `UUID` here, not sequential, so `created_at` is the real ordering column, confirmed against `V2__issuer_baseline.sql:257-267`'s actual columns, unlike `MCN-601.md`'s `AuthHoldRepository` which used a `BIGSERIAL`); `.markPublished(UUID id) -> void`.

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;

class OutboxEventRepositoryTest extends AbstractRepositoryTest {

  @Test
  void should_find_unpublished_in_created_order_then_mark_published__MCN_701_AC1() {
    OutboxEventRepository repo = new OutboxEventRepository(dataSource());
    UUID first = seedOutboxEvent("TRANSACTION", "1", "TransactionApproved", "{\"rrn\":\"111\"}");
    UUID second = seedOutboxEvent("TRANSACTION", "2", "TransactionApproved", "{\"rrn\":\"222\"}");

    List<OutboxEventRow> unpublished = repo.findUnpublished(10);

    assertThat(unpublished).extracting(OutboxEventRow::id).containsExactly(first, second);

    repo.markPublished(first);

    assertThat(repo.findUnpublished(10)).extracting(OutboxEventRow::id).containsExactly(second);
  }
}
```

Check: `seedOutboxEvent` is a new small helper added to `AbstractRepositoryTest` (`INSERT INTO outbox_event (id, aggregate_type, aggregate_id, event_type, payload) VALUES (gen_random_uuid(), ?, ?, ?, ?::jsonb) RETURNING id`) — grep whether `AbstractRepositoryTest` already has an outbox seeder from `LogAndOutboxTest` before adding a duplicate.

Run: `./gradlew test --tests OutboxEventRepositoryTest` → fails.

- [ ] **Step 2: Implement**: `findUnpublished` — `SELECT * FROM outbox_event WHERE published_at IS NULL ORDER BY created_at ASC, id ASC LIMIT ?` (uses `ix_outbox_unpublished`). `markPublished` — `UPDATE outbox_event SET published_at = now() WHERE id = ?`.

Run: `./gradlew test --tests OutboxEventRepositoryTest` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/persistence/OutboxEventRepository.java issuer-jpos/src/main/java/io/mcn/issuer/adapter/persistence/OutboxEventRow.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/persistence/OutboxEventRepositoryTest.java
git commit -m "feat(iss): OutboxEventRepository - findUnpublished ordered, markPublished (MCN-701)"
```

---

### Task 2: `OutboxRelay` — poll, publish, mark (AC1, AC2)

**Files:** `adapter/kafka/OutboxRelay.java`, `KafkaProducerFactory.java`, test

**Interfaces:** `KafkaSender interface { void send(String topic, String key, String valueJson, Map<String,String> headers) throws Exception; }` (a small port around the real `KafkaProducer<String,String>.send(...).get()` — synchronous per Global Constraints — implemented by `KafkaProducerFactory`'s producer wrapper). `OutboxRelay(OutboxEventRepository outboxEventRepository, KafkaSender kafkaSender, String topic)`; `.pollOnce() -> int` (rows published this call, for the test and for the `mcn_outbox_lag_seconds` metric Task 3 wires).

- [ ] **Step 1: Write the failing test**

```java
package io.mcn.issuer.adapter.kafka;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

import io.mcn.issuer.adapter.persistence.OutboxEventRepository;
import io.mcn.issuer.adapter.persistence.OutboxEventRow;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;

class OutboxRelayTest {

  @Test
  void should_publish_unpublished_rows_in_order_with_headers_then_mark_published__MCN_701_AC1()
      throws Exception {
    OutboxEventRepository outboxEventRepository = mock(OutboxEventRepository.class);
    KafkaSender kafkaSender = mock(KafkaSender.class);
    UUID id = UUID.randomUUID();
    OutboxEventRow row = new OutboxEventRow(id, "TRANSACTION", "1", "TransactionApproved",
        "{\"rrn\":\"123456789012\",\"eventType\":\"TransactionApproved\"}", Instant.now(), null, 0);
    when(outboxEventRepository.findUnpublished(anyInt())).thenReturn(List.of(row));
    OutboxRelay relay = new OutboxRelay(outboxEventRepository, kafkaSender, "issuer.transactions.v1");

    int published = relay.pollOnce();

    assertThat(published).isEqualTo(1);
    verify(kafkaSender).send(eq("issuer.transactions.v1"), eq("123456789012"), eq(row.payloadJson()),
        argThat((Map<String, String> headers) ->
            headers.get("event-id").equals(id.toString())
                && headers.get("event-type").equals("TransactionApproved")
                && headers.containsKey("traceparent")));
    verify(outboxEventRepository).markPublished(id);
  }

  @Test
  void should_not_mark_published_when_send_throws__MCN_701_AC1() throws Exception {
    OutboxEventRepository outboxEventRepository = mock(OutboxEventRepository.class);
    KafkaSender kafkaSender = mock(KafkaSender.class);
    UUID id = UUID.randomUUID();
    OutboxEventRow row = new OutboxEventRow(id, "TRANSACTION", "1", "TransactionApproved",
        "{\"rrn\":\"1\"}", Instant.now(), null, 0);
    when(outboxEventRepository.findUnpublished(anyInt())).thenReturn(List.of(row));
    doThrow(new RuntimeException("broker unreachable")).when(kafkaSender)
        .send(anyString(), anyString(), anyString(), anyMap());
    OutboxRelay relay = new OutboxRelay(outboxEventRepository, kafkaSender, "issuer.transactions.v1");

    relay.pollOnce();

    verify(outboxEventRepository, never()).markPublished(any());
  }
}
```

Run: `./gradlew test --tests OutboxRelayTest` → fails (package doesn't exist).

- [ ] **Step 2: Implement**: `pollOnce` calls `outboxEventRepository.findUnpublished(200)` (batch size 200, a named constant `BATCH_SIZE`, chosen so 200 TPS's worth of events clears in roughly one poll interval — Task 3 wires the poll interval), then for each row **sequentially** (Global Constraints — no parallel send): extracts `rrn` from `payloadJson` (a small local JSON parse — `com.fasterxml.jackson.databind.ObjectMapper`, already a transitive dependency via Spring/jPOS's JSON usage elsewhere, confirmed before assuming it's available), builds headers (`event-id` = row's `id.toString()`, `event-type` = row's `eventType`, `traceparent` = a fresh W3C-shaped value via `java.util.UUID`-derived trace/span ids if the payload carries no existing trace id — confirmed no existing trace-id field in `transaction-event.schema.json`, so this plan generates one at publish time, documented as this plan's own choice), calls `kafkaSender.send(topic, rrn, payloadJson, headers)`; on success, `outboxEventRepository.markPublished(row.id())`, increments a counter; on exception, logs and continues to the next row **without** marking this one published (Global Constraints — leaves it for the next poll, per Ruling 2's accepted at-least-once). Returns the count of rows successfully published this call.

Run: `./gradlew test --tests OutboxRelayTest` → PASS.

- [ ] **Step 3: Commit**

```bash
git add issuer-jpos/src/main/java/io/mcn/issuer/adapter/kafka/OutboxRelay.java issuer-jpos/src/main/java/io/mcn/issuer/adapter/kafka/KafkaProducerFactory.java issuer-jpos/src/test/java/io/mcn/issuer/adapter/kafka/OutboxRelayTest.java
git commit -m "feat(iss): OutboxRelay - poll unpublished, publish with RRN key + headers, mark-after-ack (MCN-701-AC1)"
```

---

### Task 3: Wire `80_outbox_relay.xml`, `mcn_outbox_lag_seconds` metric, Kafka Testcontainers integration test (AC1, AC2)

**Files:** `src/dist/deploy/80_outbox_relay.xml`, `build.gradle` (extend), `src/test/java/io/mcn/issuer/adapter/kafka/OutboxRelayIntegrationTest.java`

- [ ] **Step 1:** Add `org.apache.kafka:kafka-clients:<version pinned to the project's Kafka broker version in infra/docker-compose.yml>` to `build.gradle`'s dependencies and `org.testcontainers:kafka` to `testImplementation`. Note in the PR description: new dependency, purpose = issuer-side Kafka producer for the outbox relay, license Apache-2.0 (root `CLAUDE.md` §9).
- [ ] **Step 2:** Wire `80_outbox_relay.xml` following `70_hold_expiry_job.xml`'s exact `QBean`/timer shape (`MCN-601.md` Task 4) — `OutboxRelay.pollOnce()` every 500ms (`POLL_INTERVAL = Duration.ofMillis(500)`, chosen so 200 TPS × up-to-2s lag budget stays comfortably inside one poll's batch size).
- [ ] **Step 3:** Add a `mcn_outbox_lag_seconds` gauge (Micrometer, the same metrics library already wired per `docs/10-engineering-standards.md`'s observability section — grep an existing gauge registration in this codebase, e.g. in `40_txn_manager.xml`'s participants, and match the registration style) computed as `now() - oldest unpublished row's created_at` (0 when there are no unpublished rows), updated once per `pollOnce()` call.
- [ ] **Step 4: Write the failing integration test**

```java
package io.mcn.issuer.adapter.kafka;

import static org.assertj.core.api.Assertions.assertThat;
import static org.awaitility.Awaitility.await;

import io.mcn.issuer.adapter.persistence.OutboxEventRepository;
import java.time.Duration;
import java.util.Properties;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.junit.jupiter.api.Test;
import org.testcontainers.junit.jupiter.Testcontainers;
import org.testcontainers.kafka.KafkaContainer;

@Testcontainers
class OutboxRelayIntegrationTest extends AbstractRepositoryTest {

  @Test
  void should_publish_seeded_outbox_rows_to_issuer_transactions_v1_topic__MCN_701_AC1_AC2() {
    try (KafkaContainer kafka = new KafkaContainer("apache/kafka:3.8.0")) {
      kafka.start();
      OutboxEventRepository outboxEventRepository = new OutboxEventRepository(dataSource());
      seedOutboxEvent("TRANSACTION", "1", "TransactionApproved",
          "{\"rrn\":\"123456789012\",\"eventType\":\"TransactionApproved\"}");
      KafkaSender realSender = KafkaProducerFactory.newSender(kafka.getBootstrapServers());
      OutboxRelay relay = new OutboxRelay(outboxEventRepository, realSender, "issuer.transactions.v1");

      relay.pollOnce();

      Properties props = new Properties();
      props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, kafka.getBootstrapServers());
      props.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
      props.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class);
      props.put(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
      try (KafkaConsumer<String, String> consumer = new KafkaConsumer<>(props)) {
        consumer.subscribe(java.util.List.of("issuer.transactions.v1"));
        var records = consumer.poll(Duration.ofSeconds(5));
        assertThat(records.count()).isEqualTo(1);
        var record = records.iterator().next();
        assertThat(record.key()).isEqualTo("123456789012");
        assertThat(record.headers().lastHeader("event-type").value())
            .isEqualTo("TransactionApproved".getBytes());
      }
      await().atMost(Duration.ofSeconds(2))
          .until(() -> outboxEventRepository.findUnpublished(10).isEmpty());
    }
  }
}
```

Check: `KafkaProducerFactory.newSender(bootstrapServers)` is implemented as part of Task 2's `KafkaProducerFactory.java` — a thin real `KafkaProducer<String,String>` wrapper implementing `KafkaSender`, `send(...)` calling `producer.send(record).get()` (blocking for the ack, per Global Constraints).

Run: `./gradlew test --tests OutboxRelayIntegrationTest` → PASS (requires Docker for Testcontainers).

- [ ] **Step 5:** `./gradlew test spotlessCheck` clean. AC → test table:

| AC | Test(s) |
| --- | --- |
| MCN-701-AC1 | `OutboxEventRepositoryTest`, `OutboxRelayTest` (both cases), `OutboxRelayIntegrationTest` |
| MCN-701-AC2 | `OutboxRelayIntegrationTest` (lag observed near-zero after one poll at test scale; the real 200 TPS / <2s figure is verified at the Sprint 8 integration checkpoint per `SPRINT-8.md`'s exit checklist, not re-proven in this unit-scale test — noted here rather than silently claimed) |

- [ ] **Step 6:** Push, open PR `feat(iss): outbox relay - issuer.transactions.v1, ordered publish, mark-after-ack (MCN-701)`.

## Self-review

- [ ] Rows are never marked published before the Kafka ack — confirmed by the `should_not_mark_published_when_send_throws` test and by reading `pollOnce`'s implementation.
- [ ] Publish order is `created_at ASC, id ASC`, never `id` alone (the table's `id` is a random `UUID`, not sequential) — confirmed against `OutboxEventRow`'s real column types.
- [ ] Ruling 1's topic name (`issuer.transactions.v1`) and partition key (RRN) match `MCN-701-GW.md`'s restated Ruling 1 verbatim.
- [ ] No full PAN appears in any header or payload — the relay only republishes `LogAndOutbox`'s already-masked payload, never touches `card`/`pan_hash`.
