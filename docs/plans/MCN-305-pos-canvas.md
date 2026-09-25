# MCN-305 follow-up: POS screen matches the design canvas

Branch `fix/MCN-305-pos-canvas`. Lane WEB. Rebuilds `src/app/(console)/pos/**` and `src/components/pos/**`
against the POS design canvas (`POS.dc.html`, docs/01 §7.1) in Easy and Expert mode. No contract change.

## Understanding

The canvas already decides layout, copy, colours and motion (docs/02 §7.9: "POS processing state with spinner
and dots, result pop and decline shake"). The user decided the open points up front (see Rulings), so no
brainstorming step.

## Interfaces

- `components/pos/pos-model.ts` (pure, no React)
  - `DISPLAY_CARDS: readonly DisplayCard[]` with `{ cardToken, cardRef, last4, tag, tone }`. Display-only; no PAN.
  - `ENTRY_MODES: readonly { id: "chip" | "manual"; entryMode: EntryMode; de22: string }[]`
  - `SCENARIOS: readonly { id; cardToken; amount }[]`
  - `outcomeOf(tx: Transaction): Outcome`, where `Outcome` is `approved | rc51 | rc54 | rc61 | rc62 | rc91 | declined | reversalPending | reversed`
  - `kindOf(outcome): "ok" | "bad" | "rev"`
  - `techLine(tx): string`, built from the request/response MTI per type, STAN, RC and DE 38
  - `nextAmount(current, key): string`, the canvas keypad rules (10 digits max, no leading zeros, C gives "0")
- `components/pos/PosDevice.tsx`: the dark terminal with the merchant, a `role=status` screen, the Keypad, and a 60px pay button.
- `components/pos/Keypad.tsx`: 1–9, C, 0, ⌫ and physical keys. It ignores keys typed into inputs.
- `components/pos/CardPicker.tsx`: coloured tiles with `aria-pressed`. Balance comes from `useCard(cardRef)`.
- `components/pos/Segmented.tsx`: the canvas on/off segmented control. Both `TransactionTypeSelector` and the entry mode use it.
- `components/pos/ScenarioButtons.tsx`: pills with `aria-pressed`.
- `components/pos/ResultPanel.tsx`: three states (empty / processing / result). Props are `{ state, expert, … }`.
- `components/pos/pos.css`: device and tile tokens, plus keyframes (pop, shake, fade-up, dots, spin). Motion is transform and opacity only.
- `PinPad.tsx` is deleted. `pinblock.ts` stays because `security/PinBlockVisualiser` imports `buildPinBlock`.

## Tests (Vitest + Testing Library)

- `pos-model.test.ts`
  - `outcomeOf` maps APPROVED → approved and DECLINED 51/54/61/62/91 → rc codes.
  - `outcomeOf` maps other RCs → declined, and TIMED_OUT/REVERSAL_PENDING → reversalPending.
  - `techLine` renders `0200 STAN … → 0210 · RC 00 · field 38 = …` for a purchase, and 0100/0110 for a pre-auth.
  - `nextAmount` follows the canvas rules.
  - The display cards carry no PAN (no 16-digit run).
- `Keypad.test.tsx`
  - C clears to 0 and ⌫ drops a digit.
  - The physical keyboard types digits (AC1).
  - Keys typed into an `<input>` are ignored.
- `CardPicker.test.tsx`
  - Renders 6 tiles with tag, last4 and the balance from the card API.
  - Pressing a tile reports the token and sets `aria-pressed`.
- `ResultPanel.test.tsx`
  - The empty state reads "Thực hiện giao dịch đầu tiên".
  - Processing shows the Expert MUX line.
  - Approved has no tech line in Easy mode and has it in Expert mode.
  - A decline gets the shake class (AC3), and a reversal uses rev copy (AC5 statuses).
  - The steps list has 4 items and links to `/transactions/{rrn}`.
- `PosScreen.test.tsx`
  - Pay locks while processing and the screen shows the dots (AC2).
  - The purchase body has CHIP_NO_PIN and no `encryptedPinBlock`.
  - Nhập tay sends MANUAL_NO_PIN.
  - A scenario preset sets the card and amount.
  - COMPLETION posts to `/{rrn}/completions` with the RRN from the input.
  - BALANCE sends no amount.
  - An amount of 0 gives the local "Số tiền chưa hợp lệ" result with no request.

Verification:

```bash
pnpm lint && npx tsc --noEmit && pnpm test && pnpm build
```

Then take browser screenshots at 1440×1024 and 1280 in both modes, against `dev:mock` and against the real stack.

## Rulings

- **R1, PIN pad removed (MCN-305-AC4).**
  - The gateway never forwards a PIN (risk R-12), so the simulator sends `CHIP_NO_PIN` / `MANUAL_NO_PIN` with no `encryptedPinBlock`.
  - The AC4 tests ("PIN digits cleared after submit", PinPad masking) are deleted along with `PinPad`.
  - They are replaced by a test asserting that no PIN block is sent.
  - AC4 comes back when R-12 is closed.
- **R2, entry modes.**
  - Only the contract's EntryMode enum is offered: "Chạm chip" (DE 22 `052`, CHIP_NO_PIN) and "Nhập tay" (`012`, MANUAL_NO_PIN), per `gateway-go/internal/purchase/service.go`.
  - The canvas's "Quẹt từ · 901" has no enum value.
  - The canvas's chip code is `051`, which means chip with PIN.
- **R3, scenarios.**
  - The scenarios are the real-issuer outcomes from `contracts/fixtures/cards.json`: normal, low (51), blocked (62), expired (54) and limit (61).
  - "Sai mã PIN" is dropped (R-12). "Mạng chập chờn" is dropped too, because it needs the Chaos API.
  - Rev result copy still renders for TIMED_OUT, REVERSAL_PENDING and REVERSED responses.
- **R4, six cards, 3 per row.** The fixtures have 6 cards where the canvas shows 4. 1208 is tagged "Có hạn mức" and 5540 "Số dư lớn".
- **R5, pre-send STAN.** The processing Expert line omits the STAN: the gateway allocates it and returns it only with the response.
- **R6, tech line after a reversal.** It carries no RC, because a reversal after a timeout has none (see the Overview mock note).

## AC table

| AC | Test |
| --- | --- |
| MCN-305-AC1 keypad, cards, entry modes, scenarios | Keypad.test, CardPicker.test, PosScreen.test entry/scenario |
| MCN-305-AC2 processing + lock | PosScreen.test "locks pay while processing" |
| MCN-305-AC3 result, expert line, pop/shake | ResultPanel.test |
| MCN-305-AC4 PIN | Ruling R1; PosScreen.test "sends no PIN block" |
| MCN-305-AC5 reversal statuses | ResultPanel.test reversal; pos-model.test outcomeOf |

## Found while verifying against the real stack (out of lane, not fixed here)

- **Issuer card ETag.**
  - `GET /v1/cards/{cardRef}` sends an ETag built from the limits only; on the real stack it is currently `sha256("")` for every card.
  - A browser revalidates with `If-None-Match`, gets a 304, and keeps the old body, so balances go stale after a payment. This affects the POS tiles and the Cards screen.
  - Fix in `issuer-jpos` (`CardAdminController.etagFor`): hash the whole representation, or send `Cache-Control: no-store`.
- **The Card Admin API is served on :8081 only.** `cards-client` uses `NEXT_PUBLIC_API_BASE_URL` (the gateway, :8080), which answers 404 for `/v1/cards`. Tiles then show "Số dư —".
- **The gateway's CORS allows only `http://localhost:3000`.**
- **A balance inquiry on the real stack declines with RC 30** (format error). This is a gateway/issuer bug, and the POS shows it as a generic decline.
- **MockProvider starts two MSW workers under React StrictMode**, so dev:mock resolvers run twice. The POS mock handlers now replay by `Idempotency-Key` so a payment is applied once.
