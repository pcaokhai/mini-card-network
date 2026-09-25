import type { ChaosRun, ChaosScenarioId } from "@/shared/api/chaos-client";

/** Icon paths per scenario, from the canvas's `meta` table (ChaosLab.dc.html). */
export const SCENARIO_ICONS: Record<ChaosScenarioId, string> = {
  SLOW_NETWORK: "M12 7v5l3 2M12 3a9 9 0 1 0 0 18a9 9 0 0 0 0-18z",
  CONNECTION_CUT: "M4 12h5M15 12h5M10 8l4 8",
  DROP_RESPONSE: "M4 6h16M4 12h10M4 18h6",
  DUPLICATE_REQUEST: "M8 8h11v11H8zM5 16V5h11",
  ISSUER_DOWN: "M4 5h16v6H4zM4 13h16v6H4zM8 8h1M8 16h1",
  LATE_RESPONSE: "M12 8v4l2 2M12 4a8 8 0 1 0 0 16a8 8 0 0 0 0-16z",
};

export type RunVerdict = "none" | "pending" | "ok" | "discrepancy";

export function runVerdict(run: ChaosRun | undefined): RunVerdict {
  if (!run) return "none";
  if (run.status === "PASSED") return "ok";
  if (run.status === "FAILED") return "discrepancy";
  return "pending";
}

export type LedgerKey = "opening" | "approved" | "reversed" | "current" | "discrepancy";
export type LedgerValue =
  | { kind: "none" }
  | { kind: "pending" }
  | { kind: "money" | "debit" | "count"; amount: number };
export interface LedgerRow {
  key: LedgerKey;
  value: LedgerValue;
  /** Approved-transaction count shown in the row label. */
  count?: number;
  tone?: "ok" | "bad";
}

const NONE: LedgerValue = { kind: "none" };
const PENDING: LedgerValue = { kind: "pending" };

/**
 * The canvas's five "Kiểm chứng tiền" rows for the latest run (plan Rulings R2, R3): money that
 * depends on the closing total waits for the run to finish; the approved row's debit is the net
 * money the run moved (opening − closing).
 */
export function ledgerRows(run: ChaosRun | undefined): LedgerRow[] {
  if (!run) {
    return (["opening", "approved", "reversed", "current", "discrepancy"] as const).map((key) => ({ key, value: NONE }));
  }
  const opening = run.openingBalanceTotal ?? 0;
  const verdict = runVerdict(run);
  const finished = verdict === "ok" || verdict === "discrepancy";
  const closing = run.closingBalanceTotal ?? 0;
  const discrepancy = run.ledgerDiscrepancy ?? 0;
  return [
    { key: "opening", value: { kind: "money", amount: opening } },
    { key: "approved", count: run.approved ?? 0, value: finished ? { kind: "debit", amount: opening - closing } : PENDING },
    { key: "reversed", value: { kind: "count", amount: run.reversed ?? 0 } },
    { key: "current", value: finished ? { kind: "money", amount: closing } : PENDING },
    finished
      ? { key: "discrepancy", value: { kind: "money", amount: discrepancy }, tone: discrepancy === 0 && verdict === "ok" ? "ok" : "bad" }
      : { key: "discrepancy", value: PENDING },
  ];
}

const MS_PER_SECOND = 1000;
const number = (value: number, digits: number) =>
  new Intl.NumberFormat("vi-VN", { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(value);

/** "212 ms" below a second, "3,2 giây" above, as the canvas's latency tile. */
export function latencyParts(ms: number): { unit: "ms" | "s"; value: string } {
  if (ms < MS_PER_SECOND) return { unit: "ms", value: number(ms, 0) };
  return { unit: "s", value: number(ms / MS_PER_SECOND, 1) };
}
