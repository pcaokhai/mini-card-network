import type { components } from "@/shared/api/generated/schema";
import { formatMoney } from "@/shared/format/money";

export type SettlementStage = components["schemas"]["SettlementStage"];
export type TotalsRow = components["schemas"]["TotalsRow"];
export type BreakType = components["schemas"]["ReconBreak"]["breakType"];
export type Resolution = components["schemas"]["ReconBreak"]["resolution"];
export type StepStatus = "done" | "current" | "pending";
export type NextAction = "cutover" | "waitTotals" | "reconcile" | "generate";

/** Ruling R3: the contract's stage index is the canvas's `s.stage` (0–4). */
const STAGES: readonly SettlementStage[] = ["OPEN", "CUTOVER_DONE", "TOTALS_EXCHANGED", "RECONCILED", "FILE_GENERATED"];
export const STEP_KEYS = ["cutover", "totals", "reconcile", "file"] as const;

export const stageIndex = (stage: SettlementStage) => STAGES.indexOf(stage);
export const hasReached = (stage: SettlementStage, target: SettlementStage) => stageIndex(stage) >= stageIndex(target);

export function stepStatus(stage: SettlementStage, step: number): StepStatus {
  const index = stageIndex(stage);
  if (index > step) return "done";
  return index === step ? "current" : "pending";
}

/** Ruling R4/R5: step 2 is the gateway's own 0500 after the cutover; a closed day has no next step. */
const NEXT_ACTION: Record<SettlementStage, NextAction | null> = {
  OPEN: "cutover",
  CUTOVER_DONE: "waitTotals",
  TOTALS_EXCHANGED: "reconcile",
  RECONCILED: "generate",
  FILE_GENERATED: null,
};
export const nextAction = (stage: SettlementStage) => NEXT_ACTION[stage];

/** Ruling R8: the contract resolves with one of three outcomes; the canvas's actions map onto them. */
export function resolutionFor(breakType: BreakType): Exclude<Resolution, "OPEN"> {
  if (breakType === "STATUS_MISMATCH") return "AUTO_RESOLVED";
  return breakType === "DUPLICATE" ? "WRITTEN_OFF" : "MANUAL_ADJUSTED";
}

const pad = (n: number) => String(n).padStart(2, "0");

/** `dd/MM` of the business date and the next day, and the next day as DE 15 (`MMDD`). */
export function dayLabels(businessDate: string) {
  const [y = 0, m = 1, d = 1] = businessDate.split("-").map(Number);
  const next = new Date(Date.UTC(y, m - 1, d + 1));
  const nextDd = pad(next.getUTCDate());
  const nextMm = pad(next.getUTCMonth() + 1);
  return { day: `${pad(d)}/${pad(m)}`, nextDay: `${nextDd}/${nextMm}`, nextMmdd: `${nextMm}${nextDd}` };
}

export const formatCount = (n: number) => new Intl.NumberFormat("vi-VN").format(n);

/** Ruling R7: counts are plain numbers, amounts are minor units in the day's currency. */
export function formatTotal(row: TotalsRow, side: "acquirer" | "issuer", currency: string): string {
  const value = row[side];
  return row.metric.endsWith("_COUNT") ? formatCount(value) : formatMoney({ amount: value, currency });
}

export const shortHash = (sha256: string) => `${sha256.slice(0, 6)}…${sha256.slice(-4)}`;

/** Ruling R11: the business day is the viewer's local day, as the sidebar's date card shows. */
export function localBusinessDate(now = new Date()): string {
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}
