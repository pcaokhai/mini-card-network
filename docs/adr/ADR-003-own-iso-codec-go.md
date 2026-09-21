# ADR-003: Build our own ISO 8583 codec in Go, cross-checked against a library

- Status: Accepted · Date: 2026-09-21 · Related: MCN-101, docs/03

## Context
Learning the wire format is a primary goal; the issuer uses jPOS. We need byte-level certainty that both sides agree.

## Options considered
1. Use `moov-io/iso8583` directly — faster, less learning.
2. Own codec generated from `packager-spec.yaml`, with golden vectors and a test-only cross-check against moov-io.

## Decision
Option 2. Both our codec and the jPOS packager are generated from the same spec file.

## Consequences
More code to maintain (small, pure, heavily tested). Fuzzing and vectors required. We can swap to a library later behind the same interface.
