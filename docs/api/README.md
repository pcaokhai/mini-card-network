# Console API contracts: index and shared conventions

| | |
| --- | --- |
| Document | `docs/api/README.md` |
| Version | 1.0 |
| Status | Approved |
| Date | 2026-09-25 |
| Scope | Every screen of the web console (`web-next`) and the backend calls behind it |

Each console screen has one integration contract in this folder. A page contract says which endpoints the screen calls, when, what every field drives in Easy and Expert modes, the provider's rules, the errors and how the screen reacts, and what is not built yet. It complements the schemas and never overrides them.

**Normative sources, in precedence order** (root `CLAUDE.md`):
1. accepted ADRs (`docs/adr/`);
2. `contracts/openapi.yaml` and `contracts/ws-events.schema.json`;
3. `docs/03-iso8583-interface-spec.md`;
4. `docs/04-api-contract.md` (conventions, endpoint catalogue);
5. the page contracts in this folder.

When a page contract disagrees with a higher source, the higher source wins. File the conflict as a gap in that page's §9 and open a doc-fix PR.

## 1. Page contracts

| Screen | Route(s) | Contract | WEB story | Provider stories | Provider status |
| --- | --- | --- | --- | --- | --- |
| Tổng quan | `/` | [overview-page.md](overview-page.md) | MCN-306 | MCN-304, MCN-002 | Built; the switch card needs MCN-802 |
| Máy POS | `/pos` | [pos-page.md](pos-page.md) | MCN-305, MCN-604 | MCN-303, MCN-603 | Built |
| Hành trình giao dịch | `/transactions`, `/transactions/{rrn}` | [journey-page.md](journey-page.md) | MCN-307, MCN-406 | MCN-304 | Built |
| Phòng lab message | `/lab/message` | [message-lab-page.md](message-lab-page.md) | MCN-104 | MCN-103 | Built |
| Vận hành mạng | `/network` | [network-page.md](network-page.md) | MCN-205, MCN-804 | MCN-204, MCN-802 | Built; switch/STIP planned (MCN-802) |
| Phòng lab sự cố | `/lab/chaos` | [chaos-lab-page.md](chaos-lab-page.md) | MCN-405 | MCN-404 | Built |
| Thẻ và tài khoản | `/cards`, `/cards/{cardRef}` | [cards-page.md](cards-page.md) | MCN-309 | MCN-308 | Built (Issuer Admin API) |
| Bảo mật và khóa | `/security` | [security-page.md](security-page.md) | MCN-505 | MCN-504, MCN-502, MCN-503 | Built |
| Chốt ngày và đối soát | `/settlement` | [settlement-page.md](settlement-page.md) | MCN-705 | MCN-702, MCN-703, MCN-704 | **Planned (Sprint 9)**; dev:mock only |

Every page contract follows the same structure:
1. Purpose and scope
2. Page map
3. Call inventory
4. Calls
5. Real-time events
6. Security and compliance
7. Non-functional requirements
8. UI states
9. Implementation status and gaps
10. Change log

## 2. Topology

```
Browser ──REST──▶ Next.js BFF  /api/v1/**            (web-next/src/app/api/[...path]/route.ts)
                     ├─ /api/v1/cards*  ──▶ Issuer Admin API   ISSUER_ADMIN_URL (default http://localhost:8081)
                     └─ everything else ──▶ Acquirer gateway   GATEWAY_URL      (default http://localhost:8080)
Browser ──WebSocket──▶ gateway /v1/stream                      NEXT_PUBLIC_WS_URL
```

| Mode | How to run | API base in the browser |
| --- | --- | --- |
| Real stack | `make up && make seed`, then `cd web-next && pnpm dev` | `${origin}/api` (the BFF) |
| Mock | `cd web-next && pnpm dev:mock` | `${origin}/api`, answered by MSW in the browser |

**BFF forwarding rules:**
- It forwards only these request headers: `Accept`, `Content-Type`, `Idempotency-Key`, `If-Match`, `traceparent`. Cookies are dropped.
- It returns only these response headers: `Content-Type`, `ETag`, `Location`, `Retry-After`.
- Upstream failures become `502 application/problem+json` (`upstream-unavailable`).
- It adds no auth, caching or rate limiting (v1 lab).

**Mock layer:**
- MSW handlers are registered in this order:
  1. `src/mocks/pages/*.ts` (one file per page, owned with that page);
  2. `src/mocks/journey-handlers.ts`;
  3. `src/mocks/scenario-handlers.ts`;
  4. the handlers generated from `contracts/openapi.yaml`.
- Mock values mirror the design canvas (`project/<Screen>.dc.html`) and are shaped by the contract.

## 3. Shared conventions

The rules below apply to every page contract. `docs/04-api-contract.md` §2–3 is the normative text. This section adds how each rule is implemented today, so a page contract only has to record deviations.

| Topic | Rule | Implementation today |
| --- | --- | --- |
| Versioning | URL major `/v1`. Additive changes only; a breaking change needs `/v2` plus an ADR. `make contracts` runs oasdiff. | Enforced in CI (`contracts` job). |
| Media type | `application/json`; errors are `application/problem+json`. | Gateway and issuer. |
| Money | `{ "amount": <integer minor units>, "currency": "<ISO 4217 numeric>" }`, never floating point. | Everywhere. The UI formats vi-VN (`250.000 ₫`). |
| Time | RFC 3339 UTC timestamps; business dates `YYYY-MM-DD`. | Everywhere. "Today" on the Overview is the UTC day (overview G7). |
| Identifiers | Transactions are addressed by `rrn` (12 digits), cards by `cardRef`. Never a PAN. | Everywhere. |
| Card data | At most `maskedPan` (first 6 + last 4). Never PAN, CVV, PIN, PIN block, track data or clear keys. Keys by KCV only. | Enforced; `make pci-scan` runs in CI. |
| Idempotency | `Idempotency-Key` (UUID) is required on every state-changing POST/PUT/DELETE. A replay returns the stored status and body. Reusing a key with a different body gives 422. | Gateway (transactions, cancellations) and issuer (blocks, limits). A missing key gives 400. |
| Concurrency | Updatable admin resources return `ETag`; updates need `If-Match`; a mismatch gives 412. | Issuer card limits. The ETag covers the limits only (cards G-ETag). |
| Pagination | Cursor based: `?limit=&cursor=` returns `{ items, nextCursor \| null }`, with a maximum limit of 200. | Transactions list, card ledger (`cursor` = the last `journalId`). |
| Errors | RFC 9457 problem details: `type`, `title`, `status`, `detail`, `instance`, `traceId`. | **Issuer:** full URI `type` (`https://mcn.local/problems/not-found`) with `instance` and `traceId`. **Gateway:** the same shape since #131 (P-1), from one writer in `internal/api/problems.go`. |
| Declines | A declined or timed-out transaction is **not** an HTTP error. It returns 201 with `status` DECLINED, TIMED_OUT or REVERSAL_PENDING. | Gateway. |
| Tracing | `traceparent` is accepted and propagated; responses carry `X-Trace-Id`. | `traceparent` is forwarded by the BFF. The gateway doesn't return `X-Trace-Id` (P-2). |
| Actor | The BFF sends `X-Actor: <username>` for audit. | The issuer reads `X-Actor` (`CardAdminController.actor`, default `"unknown"`). The BFF doesn't send it (P-3). |
| Display | The API returns codes plus English fallback labels (`responseCode`/`responseLabel`, `JourneyStep.code`). The UI renders vi/en copy from codes. | Web i18n in `web-next/messages/{vi,en}.json`. |
| Client caching | The UI uses TanStack Query with its defaults (3 retries with backoff, refetch on focus) unless a page contract says otherwise. | `QueryProvider` is a default `QueryClient`. The settlement day query doesn't retry a 4xx. |

### Platform gaps (shared by every page)

| ID | Gap | Evidence | Owner | Fix |
| --- | --- | --- | --- | --- |
| P-1 | ~~The gateway's problem `type` is a bare slug, with no `instance` or `traceId`.~~ | `curl :8080/v1/transactions/000000000000` returns `{"type":"unknown-transaction",…}` | GW | **Fixed** in #131: `problem()` emits `https://mcn.local/problems/<slug>`, `title`, `status`, `detail`, `instance` and `traceId` as `application/problem+json`; every handler uses it, and every slug is from the catalogue |
| P-2 | The gateway doesn't return `X-Trace-Id`. | Response headers of any gateway call | GW | Set it from the request span; add it to the BFF's forwarded response headers. |
| P-3 | The BFF doesn't send `X-Actor`, so issuer audit rows read `unknown`. | `route.ts` header allow-list; `CardAdminController.actor` | WEB | Add `X-Actor` once the BFF has a session (v1 has no end-user auth). |
| P-4 | CORS on the gateway allows only `http://localhost:3000`. | `Access-Control-Allow-Origin` | GW | Not needed while every browser call goes through the BFF. Keep it or remove it, but don't widen it. |

## 4. Real-time channel

`WS /v1/stream` (schema `contracts/ws-events.schema.json`) carries:
- `transaction.created` and `transaction.updated`;
- `link.status` and `network.event`;
- `chaos.changed` and `chaos.run.progress`.

Each page contract's §5 lists the events it consumes. As of this release the gateway never emits `transaction.updated` (overview G3).

## 5. Maintaining these documents

- Update the page contract **in the same PR** as any change to an endpoint it lists or to the screen's use of it (root CLAUDE.md §7, Definition of Done).
- A contract change (`contracts/`) goes first in its own PR. The page contract follows in the provider or consumer PR.
- Each page file's §10 change log records every revision. Bump the minor version for additive changes and the major version when a call is removed or its semantics change.
- Verify examples against the running stack with GET requests only. Take examples for state-changing calls from the dev:mock handlers or the OpenAPI examples, and label them.
