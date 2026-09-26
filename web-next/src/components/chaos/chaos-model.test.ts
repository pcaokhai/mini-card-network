import { describe, expect, it } from "vitest";
import type { ChaosRun } from "@/shared/api/chaos-client";
import { newerRun } from "@/shared/api/chaos-client";
import { latencyParts, ledgerRows, runVerdict } from "./chaos-model";

const running: ChaosRun = {
  runId: "run-1",
  status: "RUNNING",
  requested: 100,
  completed: 40,
  approved: 35,
  declined: 2,
  reversed: 3,
  openingBalanceTotal: 500_000_000,
  closingBalanceTotal: 0,
  ledgerDiscrepancy: 0,
};
const passed: ChaosRun = { ...running, status: "PASSED", completed: 100, approved: 86, declined: 6, reversed: 8, closingBalanceTotal: 484_090_000 };

describe("runVerdict", () => {
  it("maps no run, running, passed and failed __MCN_405_AC2", () => {
    expect(runVerdict(undefined)).toBe("none");
    expect(runVerdict(running)).toBe("pending");
    expect(runVerdict({ ...running, status: "VERIFYING" })).toBe("pending");
    expect(runVerdict(passed)).toBe("ok");
    expect(runVerdict({ ...passed, status: "FAILED" })).toBe("discrepancy");
  });
});

describe("ledgerRows", () => {
  it("shows the five canvas rows with no values before any run __MCN_405_AC2", () => {
    const rows = ledgerRows(undefined);
    expect(rows.map((r) => r.key)).toEqual(["opening", "approved", "reversed", "current", "discrepancy"]);
    expect(rows.every((r) => r.value.kind === "none")).toBe(true);
  });

  it("shows opening and counts but pending money while a run is in flight __MCN_405_AC2", () => {
    const rows = ledgerRows(running);
    expect(rows[0]?.value).toEqual({ kind: "money", amount: 500_000_000 });
    expect(rows[1]).toMatchObject({ count: 35, value: { kind: "pending" } });
    expect(rows[2]?.value).toEqual({ kind: "count", amount: 3 });
    expect(rows[3]?.value.kind).toBe("pending");
    expect(rows[4]?.value.kind).toBe("pending");
  });

  it("derives the debit from opening minus closing and a zero discrepancy in green on PASSED __MCN_405_AC2", () => {
    const rows = ledgerRows(passed);
    expect(rows[1]?.value).toEqual({ kind: "debit", amount: 15_910_000 });
    expect(rows[3]?.value).toEqual({ kind: "money", amount: 484_090_000 });
    expect(rows[4]).toMatchObject({ value: { kind: "money", amount: 0 }, tone: "ok" });
  });

  it("marks a non-zero discrepancy red on FAILED __MCN_405_AC2", () => {
    const rows = ledgerRows({ ...passed, status: "FAILED", ledgerDiscrepancy: 500 });
    expect(rows[4]).toMatchObject({ value: { kind: "money", amount: 500 }, tone: "bad" });
  });
});

describe("latencyParts", () => {
  it("keeps sub-second latency in ms and shows seconds with one decimal above __MCN_405_AC3", () => {
    expect(latencyParts(212)).toEqual({ unit: "ms", value: "212" });
    expect(latencyParts(3200)).toEqual({ unit: "s", value: "3,2" });
  });
});

describe("run errors and snapshot order", () => {
  const runError: ChaosRun = { ...running, status: "FAILED", failureKind: "RUN_ERROR", failureDetail: "SAF not drained after the run (2 pending)" };

  it("tells an infrastructure error apart from a ledger mismatch __CHA_G3", () => {
    expect(runVerdict(runError)).toBe("error");
    expect(runVerdict({ ...passed, status: "FAILED", failureKind: "LEDGER_MISMATCH", ledgerDiscrepancy: -2000 })).toBe("discrepancy");
    expect(ledgerRows(runError).find((r) => r.key === "discrepancy")?.value).toEqual({ kind: "none" });
    expect(ledgerRows(runError).find((r) => r.key === "current")?.value).toEqual({ kind: "none" });
  });

  it("keeps the newer of two snapshots by seq __CHA_G13", () => {
    const older = { ...running, seq: 4, completed: 40 };
    const newer = { ...running, seq: 7, completed: 70 };
    expect(newerRun(newer, older)).toBe(newer);
    expect(newerRun(older, newer)).toBe(newer);
    expect(newerRun(undefined, older)).toBe(older);
    expect(newerRun({ ...running }, older)).toBe(older); // no seq on the cached one: take the incoming
  });
});
