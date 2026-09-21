# ADR-006: Store-and-forward queue in PostgreSQL with SKIP LOCKED

- Status: Accepted · Date: 2026-09-21 · Related: MCN-401, NFR-03

## Context
Reversals and advices must survive crashes and be delivered exactly once in effect (issuer idempotent).

## Options considered
1. In-memory queue — lost on crash.
2. Kafka topic — extra ordering/ack complexity for per-link delivery.
3. `saf_queue` table written in the same transaction as the state change; workers claim with `FOR UPDATE SKIP LOCKED`.

## Decision
Option 3.

## Consequences
Durable, transactional with the state change, multiple workers safe. Requires index on due rows and cleanup of ACKED rows (retention job, 30 days).
