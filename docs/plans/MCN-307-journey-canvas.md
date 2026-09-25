# MCN-307 / MCN-406 fix: the Journey screen as in the design canvas

> executing-plans, TDD per step. Pairs with GW `feat/MCN-304-journey-canvas` and contract PR #92 (`JourneyStep.code`).

**Goal:** `/transactions` stops being a "coming in R3" placeholder. It and `/transactions/{rrn}` render the canvas's Journey screen (`project/Journey.dc.html`) in Easy and Expert modes, from real data.

**Found by:** the user, 2026-09-25: "trang hành trình giao dịch vì sao vẫn chưa có gì".

## Rulings

1. **Canvas tabs.** The canvas's three scenario tabs become "the newest transaction with this outcome":
   - Thành công → `APPROVED`;
   - Bị từ chối → `DECLINED`;
   - Đã tự hủy → `REVERSED`.

   The tab lives in the URL (`?view=`). The canvas's "Hủy phải gửi lại nhiều lần" tab isn't a status; a multi-attempt reversal shows as a 0421 "Gửi lại lệnh hủy" step inside the reversed journey.
2. **Copy.** Step copy comes from `JourneyStep.code` via next-intl (vi/en). The provider's English `title`/`easyText` is only the fallback for a step without a code.
   - Expert mode shows the provider's `technicalText` in the timeline and the tech box, as the canvas does.
   - The Easy explanation stays in the detail panel in both modes.
3. **Money comes from the issuer's ledger**, through the BFF: the card is matched by masked PAN, then its current ledger balance is rewound through its journals to just before this RRN's first journal.
   - A timed-out purchase was still debited by the issuer, which the gateway never saw, so the debit and credit rows are the issuer's real journals, placed at REQUEST_SENT/ISSUER_APPROVED and REVERSAL_CONFIRMED.
   - Without the ledger, the provider's deltas are shown.
   - A decline shows no movement.
4. **BFF.** web-next/CLAUDE.md describes a BFF that was never built: the browser called the gateway directly, and `/v1/cards` (issuer admin, :8081) returned 404. `src/app/api/[...path]/route.ts` now proxies `/api/v1/cards*` to `ISSUER_ADMIN_URL` and everything else to `GATEWAY_URL`.
   - Real mode drops `NEXT_PUBLIC_API_BASE_URL` and uses `${origin}/api`.
   - The WebSocket stays direct.
5. **Motion stays on transform/opacity.** The canvas's box-shadow ring on the current step becomes a scaled, fading pseudo-element.
   - The autoplay pace is the canvas's 950 ms per step, replacing MCN-307's 2 s ruling.
   - The 30 s timeout still gets its accelerated countdown ring (MCN-406-AC2).
6. **Layout.** The step controls stay on the heading's row only from 1400px up. Below that they wrap, so the heading never collapses (it overlapped the counter at 1280px).

## Interfaces

- `useLatestTransaction(status)`: `GET /v1/transactions?status=&limit=1`.
- `useCustomerLedger(maskedPan, rrn)`: `{ currentBalance, newestFirst }`, paging the ledger until past the RRN (at most 10 × 200 journals).
- `journey-model.ts`: `outcomeOf`, `formatOffset`, `customerBalances`, `moneyRows`, `OUTCOME_TONE`, `KIND_TONE`.
- `useJourneyCopy(journey)`: `{ step(step, index), summary(outcome), expert, locale }`.
- Components: `JourneyView`, `JourneyHeader`, `SummaryHeader`, `StepTimeline`, `StepDetail`, `MoneyPanel`, `PlaybackControls`, `CountdownRing`.

## AC → tests

| AC | Test |
| --- | --- |
| MCN-307-AC1 summary, timeline, detail with fields, money | `JourneyView.test.tsx` (`__MCN_307_AC1`) |
| MCN-307-AC2 autoplay, back/forward, restart, dimmed future | `JourneyView.test.tsx` (`__MCN_307_AC2`) |
| MCN-307-AC3 `/transactions` opens a journey | `JourneyIndexScreen.test.tsx` |
| MCN-406-AC1 debit then refund | `journey-model.test.ts`, `JourneyView.test.tsx` (`__MCN_406_AC1`) |
| MCN-406-AC2 accelerated countdown | `JourneyView.test.tsx` (`__MCN_406_AC2`), `CountdownRing.test.tsx` |
| BFF routing | `src/app/api/[...path]/route.test.ts` |
| a11y (axe) | `JourneyScreen.stories.tsx` (Approved, AutoReversed, Declined) |
