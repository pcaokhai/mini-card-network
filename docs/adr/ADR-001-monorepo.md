# ADR-001: Single monorepo for all services and contracts

- Status: Accepted · Date: 2026-09-21 · Deciders: Tech Lead · Related: MCN-001

## Context
Four services in three languages share contracts (OpenAPI, ISO vectors, events) and must release feature slices together. Claude Code lanes work in parallel.

## Options considered
1. Monorepo with per-service directories and path-filtered CI — atomic contract changes, one PR history, simple local run; CI must be path-aware.
2. Repo per service + contracts repo — strong isolation; cross-repo changes and version pinning slow a small team.

## Decision
Monorepo. Lanes own directories (CODEOWNERS); CI runs per changed path; contracts live in `contracts/`.

## Consequences
One clone runs everything; git worktrees enable parallel lanes. Discipline needed to keep services decoupled (no cross-imports; enforced by build files and review).
