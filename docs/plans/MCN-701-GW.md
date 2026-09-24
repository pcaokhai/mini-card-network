# MCN-701-GW Outbox relay (gateway) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans with superpowers:test-driven-development. Worktree `../mcn-worktrees/gw-701`, branch `feat/MCN-701-gw-outbox-relay`. Requires MCN-303 merged (this plan's outbox writes hook into `purchase.Service`'s already-established `tranLog`/state-transition points). Runs in parallel with **MCN-701-ISS** (disjoint directories, disjoint DB schemas). One story (3 pts) split across two lanes — see `MCN-701-ISS.md`'s Ruling 1 for the Kafka topic/partitioning convention, authored there first (the issuer's `outbox_event` table already existed before this sprint; the gateway's does not — confirmed by grep, §"Ruling 1" below), restated here verbatim.

**Goal:** A relay publishes unpublished `acquirer.outbox_event` rows, in order, to Kafka topic `acquirer.transactions.v1` with `key=RRN` and headers `traceparent`/`event-id`/`event-type`, marking each row published only after the broker ack; `mcn_outbox_lag_seconds` stays under 2s at 200 TPS.

**Architecture:** Unlike the issuer side, `acquirer.outbox_event` **does not exist yet** — confirmed by reading `docs/assets/baseline-schema.sql`'s full `acquirer` schema section (`merchant`, `terminal`, `tran_log`, `tran_state_history`, `saf_queue`, `link_state`, `key_store`, `key_rotation`, `audit_log` — no `outbox_event`) and `docs/05-data-model.md`'s own §"acquirer" row, which lists `outbox_event (same shape as issuer)` as "MCN-703" in its Traces column — i.e. explicitly owed to this sprint, not built yet. This plan's Task 1 is therefore a real migration (`00006_outbox_event.sql`), matching `issuer.outbox_event`'s column shape exactly (`id UUID`, `aggregate_type`, `aggregate_id`, `event_type`, `payload JSONB`, `created_at`, `published_at`, `attempts`) so both lanes' relays and `MCN-703`'s Kafka consumers can share one mental model of "an outbox row" even though they're two separate tables in two separate services. A new `internal/outbox` package (`store.OutboxRepository`-style, following `internal/store/keystore.go`'s exact `pool *store.Pool` constructor-injection pattern) provides `.Insert`/`.FindUnpublished`/`.MarkPublished`, called from `purchase.Service`'s and `advtxn.Service`'s existing state-transition points (`purchase.Service.CreatePurchase` already writes `tranLog` state transitions at approve/decline; this plan adds one `outboxRepo.Insert(...)` call alongside each of those existing writes — reusing the transition points, not inventing new ones) whenever a transaction reaches a terminal or hold-related state (`TransactionApproved`/`TransactionDeclined`/`TransactionReversed`; `HoldCreated`/`HoldCompleted` map to `advtxn.Service.CreatePreAuth`/`CreateCompletion`'s equivalent transition points, confirmed against `MCN-603.md`'s already-merged `advtxn.Service` — no `HoldExpired` on the gateway side, since holds live only in `issuer.auth_hold` per `MCN-601.md`, not mirrored in the gateway's schema). `internal/outbox.Relay` (mirrors `internal/rotation.Runner`'s polling shape only insofar as both are simple synchronous loops — this one runs on a ticker, not per-HTTP-call) polls `FindUnpublished`, publishes via `github.com/segmentio/kafka-go` (a new dependency — the Go ecosystem's most common minimal Kafka client, chosen over `confluent-kafka-go` because it's pure Go with no cgo/librdkafka system dependency, simpler to containerize in this project's minimal Docker images per `docs/10-engineering-standards.md`'s "minimal container image" concern already referenced in `purchase/service.go`'s own `time/tzdata` import comment), `kafka.Writer.WriteMessages` (synchronous, waits for ack per Global Constraints), then `MarkPublished`.

**Tech Stack:** Go 1.24, `pgx`, `github.com/segmentio/kafka-go` (new dependency), existing `internal/purchase`, `internal/advtxn`, `internal/store`.

**Spec:** `docs/06-user-stories.md` MCN-701-AC1/AC2, `docs/05-data-model.md` §"acquirer" row for `outbox_event` ("same shape as issuer", traced to MCN-703), `contracts/events/transaction-event.schema.json` (the exact event shape — `source: "ACQUIRER"` for every event this relay publishes), root `CLAUDE.md` §6 rule 10 (trace propagation), rule 2 (PCI — `maskedPan` only).

## Global Constraints

- `acquirer.outbox_event`'s columns are identical in name and type to `issuer.outbox_event`'s (`id UUID PRIMARY KEY`, `aggregate_type TEXT CHECK (...)`, `aggregate_id TEXT`, `event_type TEXT`, `payload JSONB`, `created_at TIMESTAMPTZ`, `published_at TIMESTAMPTZ`, `attempts INTEGER`) — confirmed against `V2__issuer_baseline.sql:257-267` before writing the migration, not redesigned independently.
- The relay never re-orders rows (`ORDER BY created_at ASC, id ASC`) and never marks a row published before the Kafka ack — identical to `MCN-701-ISS.md`'s Global Constraints, restated here since both relays share the exact same correctness requirement.
- `payload`'s `amount`/`approvedAmount`/`balance` fields are always the `int64` minor-unit values already produced by `purchase.Service`/`advtxn.Service` — the outbox write is a direct JSON marshal of already-`int64` fields, no float ever enters the payload, per root `CLAUDE.md` §6 rule 1.
- No full PAN in any payload — `payload.maskedPan` is `obs.MaskPAN`'s existing output (`purchase.Service.CreatePurchase` already computes `maskedPAN` for its own `Transaction` — Task 2 reuses that same value, not a second masking call).

## Ruling 1: Kafka topic and partitioning convention — restated verbatim from `MCN-701-ISS.md`'s Ruling 1

Topic: `acquirer.transactions.v1` (this lane) / `issuer.transactions.v1` (the issuer lane) — two topics, one per source, matching `contracts/events/transaction-event.schema.json`'s own `title`. Partition key: the transaction's **RRN**, not STAN (per-connection only, can collide across terminals) and not `eventId` (would scatter one transaction's event sequence across partitions, breaking `MCN-703`'s per-RRN ordering need). `docker-compose.yml`'s default `num.partitions` is used as-is; no custom partitioner, since RRN-as-key with the default hash partitioner already gives per-RRN ordering. This lane's `kafka-go` `Writer` is configured with `Balancer: &kafka.Hash{}` (message key hashing, matching the Java producer's default `DefaultPartitioner` behavior on the issuer side — confirmed as the closest equivalent so both lanes' identical RRN keys land on the *same* partition-selection semantics, even though `kafka-go`'s hash function and Java's `Utils.murmur2` are not bit-identical; this does not need to be identical, only "same RRN routes deterministically within each topic", which `Hash{}` guarantees per-topic on its own).

## Ruling 2: at-least-once delivery is accepted — restated verbatim from `MCN-701-ISS.md`'s Ruling 2

A relay crash between a Kafka ack and `MarkPublished` leaves that row republished on restart — a duplicate on the topic. This plan does not add exactly-once delivery machinery (transactional outbox-to-Kafka CDC); `MCN-703.md`'s inbox pattern (dedup on `event-id`) is the story designed to absorb this gap, identically on both the issuer and gateway sides.

## File map

| Action | Path (under `gateway-go/`) |
| --- | --- |
| Create | `migrations/00006_outbox_event.sql` |
| Create | `internal/outbox/repository.go` (`Repository`), `internal/outbox/relay.go` (`Relay`), tests |
| Modify | `internal/purchase/service.go` (outbox write on approve/decline/reversal), test |
| Modify | `internal/advtxn/service.go` (outbox write on pre-auth/completion transitions), test |
| Modify | `cmd/gateway/main.go` (wire `outbox.Repository`, `outbox.Relay`, start its ticker loop) |
| Create | `internal/outbox/relay_integration_test.go` (build-tagged `integration`, real Kafka via `make up`) |
| Modify | `go.mod`, `go.sum` (add `github.com/segmentio/kafka-go`) |

---

### Task 1: `migrations/00006_outbox_event.sql`

**Files:** `migrations/00006_outbox_event.sql`

- [ ] **Step 1:** Confirm no `outbox_event` table exists yet in `gateway-go/migrations/` (`grep -rl outbox_event gateway-go/migrations/` → no matches, confirmed in this plan's own Architecture section).

```sql
-- +goose Up
CREATE TABLE outbox_event (
    id             UUID PRIMARY KEY,
    aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('TRANSACTION','CARD','CUTOVER')),
    aggregate_id   TEXT NOT NULL,
    event_type     TEXT NOT NULL,
    payload        JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ,
    attempts       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_outbox_unpublished ON outbox_event(created_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE outbox_event;
```

- [ ] **Step 2: Commit**

```bash
git add gateway-go/migrations/00006_outbox_event.sql
git commit -m "feat(gw): outbox_event table, mirrors issuer.outbox_event shape (MCN-701)"
```

---

### Task 2: `outbox.Repository` and outbox writes in `purchase.Service`/`advtxn.Service` (AC1)

**Files:** `internal/outbox/repository.go`, test; `internal/purchase/service.go` (extend), test; `internal/advtxn/service.go` (extend), test

**Interfaces:** `Row{ID uuid.UUID; AggregateType, AggregateID, EventType string; Payload json.RawMessage; CreatedAt time.Time; PublishedAt *time.Time}`. `NewRepository(pool *store.Pool) *Repository`; `.Insert(ctx, aggregateType, aggregateID, eventType string, payload any) error` (marshals `payload` to JSONB, `INSERT INTO outbox_event (id, aggregate_type, aggregate_id, event_type, payload) VALUES ($1,$2,$3,$4,$5)`, `id` generated via `github.com/google/uuid.New()` — confirm `google/uuid` is already a dependency, grep `go.mod`, reuse rather than add a second UUID library); `.FindUnpublished(ctx, limit int) ([]Row, error)` (`ORDER BY created_at ASC, id ASC`); `.MarkPublished(ctx, id uuid.UUID) error`. `EventPayload{EventID uuid.UUID; EventType, Source, BusinessDate, RRN, STAN, TransmissionDateTime, AcquirerID, TerminalID, TransactionType string; Amount int64; Currency string; Status, MaskedPAN string; OccurredAt time.Time; ResponseCode, AuthCode, OriginalRRN *string}` (matches `contracts/events/transaction-event.schema.json` field-for-field — `json` tags exactly as the schema's `properties` names).

- [ ] **Step 1: Write the failing test**

```go
package outbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepository_insertThenFindUnpublishedThenMarkPublished__MCN_701_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	require.NoError(t, repo.Insert(ctx, "TRANSACTION", "rrn-1", "TransactionApproved",
		EventPayload{RRN: "123456789012", EventType: "TransactionApproved", Source: "ACQUIRER", Amount: 1000, Currency: "704"}))

	rows, err := repo.FindUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].PublishedAt)

	require.NoError(t, repo.MarkPublished(ctx, rows[0].ID))

	rows, err = repo.FindUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, rows)
}
```

Run: `go test ./internal/outbox/... -v` → fails (package doesn't exist).

- [ ] **Step 2: Implement** `repository.go` as described above.

Run: `go test ./internal/outbox/... -v` → PASS.

- [ ] **Step 3: Write the failing test for `purchase.Service` outbox writes**

```go
func TestCreatePurchase_writesTransactionApprovedToOutboxOnApproval__MCN_701_AC1(t *testing.T) {
	fakeOutbox := &fakeOutboxRepo{}
	svc := newTestService(t, withOutbox(fakeOutbox)) // extends purchase's existing newTestService builder

	txn, err := svc.CreatePurchase(context.Background(), samplePurchaseRequest(), "idem-outbox-1")

	require.NoError(t, err)
	require.Equal(t, "APPROVED", txn.Status)
	require.Len(t, fakeOutbox.inserted, 1)
	require.Equal(t, "TransactionApproved", fakeOutbox.inserted[0].eventType)
	require.Equal(t, txn.RRN, fakeOutbox.inserted[0].payload.(outbox.EventPayload).RRN)
}
```

Check: `fakeOutboxRepo`, `withOutbox` are small test doubles added to `purchase/service_test.go`, matching the package's existing `fakeMux`/`withMux` conventions. `Service` gains an `outbox OutboxWriter` field (`OutboxWriter interface{ Insert(ctx, aggregateType, aggregateID, eventType string, payload any) error }`, satisfied by `*outbox.Repository`) via `NewService`'s existing constructor, extended with one new trailing parameter (confirm the real current parameter list from `MCN-504-GW.md`'s already-merged signature before appending).

Run: `go test ./internal/purchase/... -run Outbox` → fails.

- [ ] **Step 4: Implement**: at `CreatePurchase`'s existing approve/decline/reversal transition points (grep exactly where `Transaction.Status` is finalized to `APPROVED`/`DECLINED`/`REVERSED`), add one `s.outbox.Insert(ctx, "TRANSACTION", txn.RRN, eventTypeFor(txn.Status), outbox.EventPayload{...})` call each, mapping `Transaction`'s already-computed fields (`RRN`, `STAN`, `Amount`, `MaskedPAN`, `ResponseCode`, etc.) 1:1 into `EventPayload` — `Source: "ACQUIRER"` always (this is the gateway/acquirer side). `eventTypeFor` maps `APPROVED→"TransactionApproved"`, `DECLINED→"TransactionDeclined"`, `REVERSED→"TransactionReversed"` (a small `switch`, matching `contracts/events/transaction-event.schema.json`'s `eventType` enum exactly).

Run: `go test ./internal/purchase/... -v` → PASS.

- [ ] **Step 5:** Repeat Steps 3–4 for `advtxn.Service.CreatePreAuth`/`CreateCompletion` — `eventTypeFor` extended with `PREAUTH→"HoldCreated"`, `COMPLETION→"HoldCompleted"` (matching the schema's `eventType` enum's hold-specific values), `TransactionType` set to `"PREAUTH"`/`"COMPLETION"` respectively. One test per flow (`TestCreatePreAuth_writesHoldCreatedToOutbox__MCN_701_AC1`, `TestCreateCompletion_writesHoldCompletedToOutbox__MCN_701_AC1`), following the same fake/assertion shape as Step 3.

Run: `go test ./internal/advtxn/... -v` → PASS.

- [ ] **Step 6: Commit**

```bash
git add gateway-go/internal/outbox/repository.go gateway-go/internal/outbox/repository_test.go gateway-go/internal/purchase/service.go gateway-go/internal/purchase/service_test.go gateway-go/internal/advtxn/service.go gateway-go/internal/advtxn/service_test.go
git commit -m "feat(gw): outbox.Repository, outbox writes at purchase/advtxn transition points (MCN-701-AC1)"
```

---

### Task 3: `outbox.Relay` — poll, publish, mark (AC1, AC2)

**Files:** `internal/outbox/relay.go`, test

**Interfaces:** `KafkaSender interface{ WriteMessage(ctx context.Context, key, value []byte, headers map[string]string) error }` (a thin port around `*kafka.Writer.WriteMessages`, implemented by `kafkaWriterAdapter` in `cmd/gateway/main.go` — kept as a narrow interface here rather than depending on `kafka-go`'s concrete `Writer` type directly in `internal/outbox`, matching `internal/purchase`'s existing `MuxSender` port-not-concrete-type convention). `Relay{repo *Repository; sender KafkaSender; topic string}`; `NewRelay(repo *Repository, sender KafkaSender, topic string) *Relay`; `.PollOnce(ctx context.Context) (int, error)`.

- [ ] **Step 1: Write the failing test**

```go
package outbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	sent    []sentMessage
	failNow bool
}

type sentMessage struct {
	key, value []byte
	headers    map[string]string
}

func (f *fakeSender) WriteMessage(ctx context.Context, key, value []byte, headers map[string]string) error {
	if f.failNow {
		return errTestSendFailure
	}
	f.sent = append(f.sent, sentMessage{key, value, headers})
	return nil
}

func TestRelay_pollOnce_publishesUnpublishedInOrderWithHeadersThenMarks__MCN_701_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	require.NoError(t, repo.Insert(ctx, "TRANSACTION", "1", "TransactionApproved",
		EventPayload{RRN: "111111111111", EventType: "TransactionApproved", Source: "ACQUIRER"}))
	require.NoError(t, repo.Insert(ctx, "TRANSACTION", "2", "TransactionApproved",
		EventPayload{RRN: "222222222222", EventType: "TransactionApproved", Source: "ACQUIRER"}))
	sender := &fakeSender{}
	relay := NewRelay(repo, sender, "acquirer.transactions.v1")

	published, err := relay.PollOnce(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, published)
	require.Equal(t, "111111111111", string(sender.sent[0].key))
	require.Equal(t, "222222222222", string(sender.sent[1].key))
	require.Equal(t, "TransactionApproved", sender.sent[0].headers["event-type"])
	require.NotEmpty(t, sender.sent[0].headers["traceparent"])

	remaining, err := repo.FindUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, remaining)
}

func TestRelay_pollOnce_leavesRowUnpublishedWhenSendFails__MCN_701_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	require.NoError(t, repo.Insert(ctx, "TRANSACTION", "1", "TransactionApproved",
		EventPayload{RRN: "111111111111", EventType: "TransactionApproved", Source: "ACQUIRER"}))
	sender := &fakeSender{failNow: true}
	relay := NewRelay(repo, sender, "acquirer.transactions.v1")

	_, err := relay.PollOnce(ctx)
	require.Error(t, err)

	remaining, err := repo.FindUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
}
```

Run: `go test ./internal/outbox/... -run Relay` → fails.

- [ ] **Step 2: Implement**: `PollOnce` calls `repo.FindUnpublished(ctx, 200)`, then for each row sequentially: unmarshal `Row.Payload` into `EventPayload` to extract `RRN`, build headers (`event-id`: `row.ID.String()`, `event-type`: `row.EventType`, `traceparent`: a freshly generated W3C-shaped value via `crypto/rand`-backed trace/span ids, matching `MCN-701-ISS.md`'s identical choice), call `sender.WriteMessage(ctx, []byte(rrn), row.Payload, headers)`; on success, `repo.MarkPublished(ctx, row.ID)`, increment count; on error, stop processing further rows in this batch and return the error with the count published so far (a send failure likely means the broker is down for *all* subsequent rows too — stopping early avoids N wasted round-trips this poll, retried whole on the next tick).

Run: `go test ./internal/outbox/... -v` → PASS.

- [ ] **Step 3: Commit**

```bash
git add gateway-go/internal/outbox/relay.go gateway-go/internal/outbox/relay_test.go
git commit -m "feat(gw): outbox.Relay - poll, publish with RRN key + headers, mark-after-ack (MCN-701-AC1)"
```

---

### Task 4: `cmd/gateway/main.go` wiring, real Kafka sender, integration test, `mcn_outbox_lag_seconds` (AC1, AC2)

**Files:** `cmd/gateway/main.go` (extend), `internal/outbox/kafka_sender.go` (new, real `kafka-go` adapter), `internal/outbox/relay_integration_test.go`, `go.mod`

- [ ] **Step 1:** `go get github.com/segmentio/kafka-go` — note in the PR description: new dependency, purpose = gateway-side Kafka producer for the outbox relay, license Apache-2.0 (root `CLAUDE.md` §9).
- [ ] **Step 2:** Implement `internal/outbox/kafka_sender.go`: `kafkaWriterSender{writer *kafka.Writer}` implementing `KafkaSender`, constructed with `&kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll}` (Ruling 1's `Hash{}` balancer, `RequireAll` for the synchronous-ack-before-mark Global Constraint), `WriteMessage` calls `writer.WriteMessages(ctx, kafka.Message{Key: key, Value: value, Headers: headersToKafka(headers)})`.
- [ ] **Step 3:** Add a `mcn_outbox_lag_seconds` gauge (the same metrics library `internal/obs` already wires — grep its existing gauge registration pattern, e.g. from MCN-407's chaos metrics, and match it) computed each `PollOnce` as `time.Since(oldest unpublished row's CreatedAt)` (0 when `FindUnpublished` returns empty).
- [ ] **Step 4:** Wire `cmd/gateway/main.go`: `outboxRepo := outbox.NewRepository(pool)`, pass `outboxRepo` into `purchase.NewService(...)`/`advtxn.NewService(...)`'s new trailing parameters (Task 2), construct the Kafka sender, `relay := outbox.NewRelay(outboxRepo, kafkaSender, "acquirer.transactions.v1")`, start a `time.NewTicker(500 * time.Millisecond)` loop in its own goroutine calling `relay.PollOnce(ctx)` each tick (mirrors `MCN-701-ISS.md`'s issuer-side 500ms poll interval — restated as the same value on both lanes so neither side's lag characteristics differ for the same reason).
- [ ] **Step 5: Write the integration test** (build-tagged, against `make up`'s real Kafka)

```go
//go:build integration

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/outbox"
)

func TestRelay_pollOnce_publishesToRealKafka_acquirerTransactionsV1__MCN_701_AC1_AC2(t *testing.T) {
	pool := newIntegrationPool(t) // against the real running Postgres, per docs/06 "make up"
	repo := outbox.NewRepository(pool)
	ctx := context.Background()
	require.NoError(t, repo.Insert(ctx, "TRANSACTION", "rrn-int-1", "TransactionApproved",
		outbox.EventPayload{RRN: "999999999999", EventType: "TransactionApproved", Source: "ACQUIRER"}))

	sender := outbox.NewKafkaWriterSender([]string{"localhost:9092"}, "acquirer.transactions.v1")
	relay := outbox.NewRelay(repo, sender, "acquirer.transactions.v1")

	published, err := relay.PollOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, published)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"localhost:9092"}, Topic: "acquirer.transactions.v1", MinBytes: 1, MaxBytes: 10e6,
	})
	defer reader.Close()
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	msg, err := reader.ReadMessage(readCtx)
	require.NoError(t, err)
	require.Equal(t, "999999999999", string(msg.Key))
}
```

Check: `newIntegrationPool`, `outbox.NewKafkaWriterSender` are adjusted to whatever real integration-test helpers `internal/rotation/runner_integration_test.go` (`MCN-504-GW.md` Task 6, already merged) established — reuse that pattern rather than inventing a second one.

Run (against `make up`): `go test -tags=integration ./internal/outbox/... -v` → PASS.

- [ ] **Step 6:** `go test ./... -race` and `make -C gateway-go lint test` clean.
- [ ] **Step 7: Commit**

```bash
git add gateway-go/cmd/gateway/main.go gateway-go/internal/outbox/kafka_sender.go gateway-go/internal/outbox/relay_integration_test.go gateway-go/go.mod gateway-go/go.sum
git commit -m "feat(gw): wire outbox.Relay in main.go, real Kafka sender, mcn_outbox_lag_seconds gauge (MCN-701-AC1/AC2)"
```

- [ ] **Step 8:** Push, open PR `feat(gw): outbox relay - acquirer.transactions.v1, ordered publish, mark-after-ack (MCN-701)`.

## Self-review

- [ ] `acquirer.outbox_event`'s columns match `issuer.outbox_event`'s exactly — confirmed by diffing the two `CREATE TABLE` statements side by side.
- [ ] Ruling 1 (topic names, RRN partition key) is identical in wording to `MCN-701-ISS.md`'s Ruling 1 — no drift between the two plans' own restatement.
- [ ] `PollOnce` never marks a row published before `WriteMessage` returns successfully — confirmed by the `leavesRowUnpublishedWhenSendFails` test and by reading the implementation.
- [ ] Every outbox write's `Amount`/`ApprovedAmount`/`Balance` field is copied from an already-`int64` source (`Transaction`/`advtxn.Transaction`'s existing fields) — no float conversion introduced in the payload-building code.
