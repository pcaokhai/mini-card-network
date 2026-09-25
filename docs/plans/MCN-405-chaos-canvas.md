# MCN-405 · Chaos Lab matches the design canvas

Story: MCN-405 (docs/06), motion: docs/02 §7.9 ("chaos card highlight and money verification flash").
Canvas: `ChaosLab.dc.html` (markup, `renderVals()`, keyframes `mcnFadeUpA`, `mcnFlashGA/GB`, `mcnBeat`).
Lane WEB only. No contract change.

## Understanding

The screen is a title row (heading, subtitle, active-count pill, "Tắt tất cả", "Chạy thử 100 giao dịch"),
a 2-column grid of six scenario cards beside a 420 px column with "Kiểm chứng tiền" (five ledger rows)
and "Hệ thống đang phản ứng thế nào" (SAF + latency tiles, calm text or one reaction per active scenario).
Expert mode swaps the ledger labels, tile labels and reaction text, and adds a mono tech line per card.

## Interfaces

- `src/components/chaos/chaos-model.ts`
  - `SCENARIO_ICONS: Record<ChaosScenarioId, string>` (canvas SVG paths)
  - `ledgerRows(run: ChaosRun | undefined): LedgerRow[]` with `LedgerRow = { key, value: LedgerValue, tone? }`,
    `LedgerValue = { kind: "money" | "count" | "debit" | "none" | "pending", amount?: number }`
  - `runVerdict(run): "none" | "pending" | "ok" | "discrepancy"`
  - `latencyParts(ms: number): { unit: "ms" | "s", value: number }`
- `ScenarioCard({ scenario, expert, busy, onToggle })` — button with `aria-pressed`
- `MoneyVerificationPanel({ run, expert })`
- `ReactionPanel({ activeScenarios, expert, safDepth, p99LatencyMs })`
- `ChaosLabScreen` wires `useChaosScenarios`, `useSetChaosScenario`, `useStartChaosRun`, `useChaosRun`,
  `useSafQueue` (network-client), `useOverview` (overview-client) and WS `chaos.run.progress`.
- `chaos-client.ts`: `useDisableAllChaosScenarios()` (PUT enabled=false for every enabled id).
- `src/mocks/pages/chaos.ts`: stateful scenarios (DROP_RESPONSE on, as the canvas), a run that
  progresses over ~3 s with the canvas's rates (declined 6 %, reversed drop 8 / cut 12 / late 5 %,
  185.000 ₫ each, opening 500.000.000 ₫).

## Tests (AC id in each name)

- `chaos-model.test.ts`: ledger rows before a run are "none"; RUNNING has opening + counts and pending money;
  PASSED gives debit = opening − closing, closing, discrepancy 0 with tone ok; FAILED tone bad (AC2);
  latency 212 → ms, 3200 → s (AC3).
- `ScenarioCard.test.tsx`: off → "Bật sự cố", click calls onToggle(id, true); on → pressed + "Đang bật · bấm để tắt",
  data-on; expert shows the tech line, easy does not (AC1).
- `MoneyVerificationPanel.test.tsx`: five canvas rows in easy and expert labels; PASSED flashes the zero row
  and shows "Sổ sách khớp"; FAILED shows the run id in red (AC2).
- `ReactionPanel.test.tsx`: calm text when nothing active; one reaction per active id in canvas order,
  easy vs expert text; tile labels per mode (AC3).
- `ChaosLabScreen.test.tsx` (MSW with the page mock): six cards and "1 sự cố đang bật"; toggling sends PUT;
  "Tắt tất cả" turns all off; "Chạy thử 100 giao dịch" posts 100 and the panel reaches PASSED (AC1–AC3).

## Rulings

- R1 Card title, description, tech line and reaction copy come from `messages/*.json` (the canvas copy, in vi and en),
  keyed by scenario id; the API's single-language `easyText`/`technicalText` are not shown.
- R2 Runs are independent 100-transaction runs, not the canvas's cumulative batches: the panel shows the latest run
  started from this page. There is no list-runs endpoint, so before the first run the rows show "—" and the badge
  reads "Chưa chạy thử".
- R3 The approved row's amount is `opening − closing` (the net money the run moved), shown once the run finishes;
  while RUNNING/VERIFYING money rows show "…" and the badge reads "Đang kiểm chứng…". FAILED shows "Sổ sách lệch",
  the discrepancy in red and "Lệch trong lần chạy {runId}".
- R4 SAF depth and latency come from the real `/v1/network/saf` depth and `/v1/metrics/overview` p99, not
  from the active scenarios.
- R5 The header link pill belongs to the shared Header and shows the real link status; it does not follow chaos toggles.
- R6 The canvas's background-colour flash (`mcnFlashGA`) becomes an overlay whose opacity animates (docs/02 §7.9);
  it replays for each finished run (keyed by run id).
- R7 "Chạy thử 100 giao dịch" is disabled while a run is RUNNING/VERIFYING, so runs never overlap.
- R8 A failed toggle or run start shows a one-line alert under the title row.

## AC table

| AC | Tests |
| --- | --- |
| MCN-405-AC1 | ScenarioCard.test, ChaosLabScreen.test (six cards, count pill, toggle, turn all off) |
| MCN-405-AC2 | chaos-model.test, MoneyVerificationPanel.test, ChaosLabScreen.test (run to PASSED) |
| MCN-405-AC3 | chaos-model.test (latency), ReactionPanel.test |

Verification: `pnpm lint && npx tsc --noEmit && pnpm test && pnpm build`, storybook/axe via `pnpm test:stories`,
Playwright screenshots at 1440×1024 and 1280 against dev:mock and the real stack (read-only).
