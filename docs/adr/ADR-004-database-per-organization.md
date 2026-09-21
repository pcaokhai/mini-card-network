# ADR-004: One database per organization (acquirer, issuer, settlement)

- Status: Accepted · Date: 2026-09-21 · Related: docs/05

## Context
In reality acquirer and issuer are different companies; reconciliation exists because their books are independent.

## Decision
Separate databases and roles; no cross-database reads. Data moves only via ISO, REST or Kafka events.

## Consequences
Realistic reconciliation and failure modes; slightly more infrastructure. Reporting across hosts goes through settlement.
