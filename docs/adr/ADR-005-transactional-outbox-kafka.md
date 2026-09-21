# ADR-005: Transactional outbox to Kafka; idempotent inbox consumers

- Status: Accepted · Date: 2026-09-21 · Related: MCN-701, MCN-703

## Context
Settlement needs every final transaction state from both hosts without dual-write inconsistencies.

## Options considered
1. Publish to Kafka inside the request path — dual write, lost or phantom events.
2. CDC (Debezium) — robust but heavier to operate for a laptop lab.
3. Outbox table written in the business transaction + relay; inbox table on the consumer.

## Decision
Option 3. At-least-once delivery, key = RRN, consumers dedupe by event id in the same DB transaction as their effects.

## Consequences
Relay lag must be monitored (`mcn_outbox_lag_seconds`). Consumers must tolerate out-of-order events per RRN.
