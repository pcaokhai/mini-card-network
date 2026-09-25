# MCN-309 — Cards and accounts as in the design canvas

Story: `docs/06-user-stories.md` MCN-309. Canvas: `Cards.dc.html` (markup, `renderVals()`, keyframes). Motion: `docs/02-software-architecture.md` §7.9 "card lock overlay".
Branch `fix/MCN-309-cards-canvas`. Lane WEB only. No contract change.

## Understanding

The canvas is one screen: the card list (320 px) beside the selected card's detail. The detail is the card visual with the lock overlay and the status panel on top, then balance and limits side by side, then the ledger. The earlier screens split this over two stacked pages with generic tables. This change rebuilds both routes on one `CardsScreen` and follows the pattern of the POS, Journey and Overview rebuilds: page-local CSS with data-attribute tones, `useDisplayMode`, next-intl, and motion on transform and opacity only.

## Interfaces

- `src/app/(console)/cards/CardsScreen.tsx`: `CardsScreen({ cardRef?: string })`. `/cards` and `/cards/[cardRef]` both render it.
- `src/components/cards/cards-model.ts`:
  - `statusKind(card, today): "active" | "locked" | "expired"`
  - `activeHoldTotal(holds)`
  - `usagePercent(used, limit)`
  - `cardTag(maskedPan)`
  - `ledgerRow(entry): LedgerRow`
  - `LIMIT_RANGES`
- Components: `CardList`, `CardVisual`, `StatusBadge`, `CardStatusPanel`, `BalancePanel`, `LimitsPanel`, `LedgerPanel`, `CardDetail` (container), `problemDetail`. Styles are in `cards.css`.
- `src/mocks/pages/cards.ts`:
  - `cardsHandlers` gains block (POST), unblock (DELETE) and limits (PUT, with a versioned `ETag` / `If-Match` and a 412 path).
  - Also exports `resetCardsMock`, `mockCardSummaries`, `mockCardDetail` and `mockLedger`.
- `src/shared/api/cards-client.ts`: after a block or unblock, invalidates all `["cards"]` queries so the list badge updates too. Hook signatures are unchanged.

## Tests

- `cards-model.test.ts`:
  - The status kind covers issuer ACTIVE past its expiry month, EXPIRED and BLOCKED/LOST.
  - Only ACTIVE holds are summed.
  - The usage percent is rounded and capped, and an unset (0) limit reads as 0%.
  - The masked PAN maps to its colour tag.
  - Each ledger row gets the right sign, legs and amount; an unposted journal has no legs; a generated description is dropped.
- `CardsScreen.test.tsx`, run against `cardsHandlers`:
  - MCN-309-AC1:
    - The list is ordered by last four digits, shows a badge per card, and the default selection is the issuer's first card.
    - The detail shows the visual, the three balance lines with the hold and its release date, and the slider at 10 000 000 ₫ with "9% hạn mức". The ledger has five rows with signed amounts.
    - In Expert mode the labels switch to the technical names and the ledger legs are shown.
  - MCN-309-AC2:
    - Blocking needs the inline confirmation. The overlay "Thẻ đang bị khóa", the "Đã khóa" badge in the panel and the list, and an audit line with HH:mm all appear.
    - Cancel leaves the card untouched.
    - An expired card has no toggle.
  - MCN-309-AC3:
    - Releasing a slider sends one PUT with `If-Match: "v1"` and both limits.
    - A 412 shows "Someone changed this card. Reload to continue." and a Reload button that restores the saved value.
  - Real issuer ledger: "PURCHASE journal 244" renders as "Mua hàng".
- `CardsScreen.stories.tsx` has six stories (Easy, Expert, Blocked Easy/Expert, Expired, Declines), each run through axe.

## Rulings

- **R1, one screen for both routes.** `/cards` renders the list with the issuer's first card selected (4417, as in the canvas's default). `/cards/[cardRef]` selects that card. List items are links with `aria-current="page"` instead of the canvas's `aria-pressed` buttons, so a card detail has a URL.
- **R2, list order.** The canvas renders its cards sorted by last four digits, because its data object uses numeric-like keys. The screen sorts the same way. It lists all six fixture cards; the canvas shows four.
- **R3, expired by date.** The issuer still reports 7765 (expiry 08/26) as ACTIVE. The screen shows it expired from the expiry month, as the canvas does. In Expert mode the badge reads `EXPIRED`.
- **R4, block reason.** The canvas has no reason picker. The API requires a reason, so the screen sends `CUSTOMER_REQUEST`. The earlier reason selector is gone.
- **R5, audit line.** The issuer writes `audit_log` but has no read API, so the line echoes this session's action with the real HH:mm. The canvas has a fixed "14:42".
- **R6, limits save on release.** The canvas's sliders have no save button. A PUT goes out on pointer-up or key-up. Saving on every `input` event would 412 against its own first write. An issuer limit of 0 (no `card_limit` row) shows "Chưa đặt" with no usage percentage.
- **R7, ledger legs in Expert mode.**
  - The canvas shows "Nợ KH 0012345678". The API's account for the customer is `ACC-<cardRef>`, and the Journey money panel keys on that prefix, so the screen shows the account as returned.
  - When the issuer only generated a description ("PURCHASE journal 244"), the screen names the entry type instead ("Mua hàng").
- **R8, rows that post nothing.** The canvas lists declines and "báo mất thẻ" with 0 ₫ ("Không hạch toán"). The real issuer posts no journal for these. dev:mock serves them as journals without postings, and the screen renders any such journal the canvas way.
- **R9, dev:mock values follow the Cards canvas.**
  - These values change:
    - 4417's ledger balance is 5 000 000 ₫, with a 1 500 000 ₫ hold, so 3 500 000 ₫ is available.
    - Holder names have diacritics.
    - The limits and "used today" are the canvas's values.
  - Because of that, the POS tile for 4417 shows 3 500 000 ₫ available in dev:mock. The Journey money panel for 626514000123 reads 5 250 000 → 5 000 000, because that purchase is the newest 4417 journal in the Cards canvas.
  - The Journey and Cards canvases disagree on 4417's balance. The cardRefs, masked PANs, statuses and GET shapes are unchanged.
- **R10, motion.**
  - The canvas's usage bar animates `width` and its card animates `background-color`. The screen animates the bar as `scaleX`, and the card background never changes.
  - The overlay (`fade .3s` plus icon `pop .45s` after `.1s`), the confirm box (`fade-up .22s`) and the audit line (`fade-up .3s`) use the canvas timings on transform and opacity.
- **R11, line-height and range margin.** The canvas sets no line-height (browser `normal`) and keeps Chrome's 2 px range margin. Preflight changes both, so `.cards-root` restores them. Every text node then sits within 1 px of the canvas at 1440.
- **R12, 1280 px.** The canvas is 1440 only. Below that, the Expert ledger legs shrink and wrap before the description does, and the available balance wraps under its label. There is no horizontal overflow.

- **R13, holds shown.** The canvas lists holds as "auth_hold ACTIVE", each with an auto-release date. The earlier screen also listed COMPLETED, RELEASED and EXPIRED holds with a status pill. The screen now lists only ACTIVE holds, the same set the "Đang tạm giữ" line sums.
- **R14, ledger length.**
  - The canvas shows a short ledger, but on the real stack a busy card has dozens of journals.
  - The screen fetches `limit=8` and shows the newest 8. A "Xem thêm" button ("Load more" in Expert mode, "Show more" in English Easy mode) fetches the next page with the issuer's `nextCursor`, and appears only while `nextCursor` is set. This is `useCardLedgerPages(cardRef, pageSize)`, an infinite query; `useCardLedger` is unchanged.
  - dev:mock pages the same way the issuer does: `cursor` is the last journalId, and `nextCursor` is set while a full page came back.

## AC table

| AC | Covered by |
| --- | --- |
| MCN-309-AC1 list with badges; visual, three-line balance, holds, sliders with usage bar, ledger | `CardsScreen.test.tsx` (the first three AC1 tests), `cards-model.test.ts`, stories |
| MCN-309-AC2 inline confirm, animated lock overlay, audit line | `CardsScreen.test.tsx` (the AC2 tests), `cards.css` `cards-lock` / `cards-pop` |
| MCN-309-AC3 If-Match, 412 message | `CardsScreen.test.tsx` (the AC3 tests), mock PUT with ETag versioning |

Dependencies: none added.
