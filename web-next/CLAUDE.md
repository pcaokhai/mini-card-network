# web-next — CLAUDE.md

POS simulator and operations console. Next.js App Router acts as BFF: Route Handlers call the gateway and the Issuer Admin API; the browser talks only to Next.js (REST) and to the gateway WebSocket with a short-lived token.

Lane: **WEB**. Owns: `web-next/**`. Design reference: the canvas linked in `docs/01-prd.md` §7.1. Motion system: `docs/02-software-architecture.md` §7.9.

## Commands

```bash
pnpm -C web-next dev            # uses MSW mocks when NEXT_PUBLIC_API_MOCKS=true
pnpm -C web-next generate       # openapi-typescript + MSW handlers from contracts/
pnpm -C web-next test           # Vitest + Testing Library
pnpm -C web-next lint           # eslint + tsc --noEmit + prettier --check
pnpm -C web-next storybook
pnpm -C web-next e2e            # Playwright (needs `make up`)
```

## Layout

```
src/app/(console)/        routes: overview, pos, transactions/[rrn], lab/message, lab/chaos,
                          cards, security, settlement, network
src/app/api/              BFF route handlers (server only)
src/features/<area>/      components/, hooks/, api/ (typed calls), schemas/ (zod), model/ (view mapping)
src/shared/ui/            design-system components (shadcn-based): StatusBadge, MaskedPan, Term, Money, Stepper…
src/shared/motion/        tokens.ts, variants.ts, useMotionPreference.ts
src/shared/realtime/      WebSocket client (reconnect, backoff, rAF batching), event bus
src/shared/api/           generated OpenAPI types + openapi-fetch client, problem+json parser
src/shared/i18n/          vi.json, en.json, glossary.json (easy label + technical label per term)
src/mocks/                MSW handlers generated from contracts + scenario fixtures
```

## Rules

- **Server Components by default.** Add `'use client'` only on the smallest interactive leaf. No data fetching in client components except through feature hooks (TanStack Query).
- **Typed boundaries.** API types are generated from `contracts/openapi.yaml`; never hand-write response types. Validate WebSocket payloads with the generated Zod schemas before use.
- **State:** server state in TanStack Query, UI state (mode toggle, POS keypad, filters) in Zustand or component state. No duplicated server data in stores.
- **Parallel with backend:** build against MSW mocks and scenario fixtures first; switching to the real API is `NEXT_PUBLIC_API_MOCKS=false`.
- **Two display modes.** Every technical term goes through `<Term id="rrn" />`; every screen works in both Easy and Expert mode. Snapshot/story for each mode.
- **Card data:** the browser never receives a full PAN. Render cards only through `<MaskedPan last4=… />`. The PIN pad encrypts in the client simulator before sending; PIN digits never enter React state that outlives the submit.
- **Money:** amounts are integer minor units in API and state; format only at render with `<Money />` (`Intl.NumberFormat('vi-VN')`).
- **Motion:** use tokens from `shared/motion/tokens.ts` only; animate `transform`/`opacity`; every animation has a reduced-motion variant via `useMotionPreference()`; realtime lists batch updates per animation frame and cap at 30 visible rows.
- **Accessibility:** real `<button>`/`<a>`/`<label>`, visible focus, 4.5:1 contrast, `aria-live="polite"` for transaction results, keyboard support for POS keypad.
- **Naming:** components `PascalCase.tsx`, hooks `useThing.ts`, other files `kebab-case.ts`. Named exports only (except Next.js route files).
- TypeScript `strict`, `noUncheckedIndexedAccess`; no `any`, no non-null `!` without a comment explaining why it is safe.

## Tests

- Unit/component: Vitest + Testing Library, test behavior not implementation; each AC referenced in the test name.
- Storybook story per shared component and per screen state (empty, loading, success, declined, timeout, reversed; Easy/Expert).
- Playwright journeys in `e2e/` for the six critical flows listed in `docs/08-test-strategy.md`; animations disabled in E2E.
