# ADR-002: Contract-first development with generated code and mocks

- Status: Accepted · Date: 2026-09-21 · Related: docs/04, docs/07 §2

## Context
Frontend and backend must be developed at the same time and released together per slice.

## Options considered
1. Code-first (derive OpenAPI from backend) — frontend waits for backend.
2. Contract-first: spec PR first; generate Go strict server, TS client and MSW handlers; validate provider responses in tests.

## Decision
Contract-first. `contracts/` is the source of truth; breaking changes blocked by oasdiff unless labelled with an ADR.

## Consequences
WEB never blocks on BE; integration surprises move to contract review. Specs must be kept precise (examples, enums, nullability).
