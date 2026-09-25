import { describe, expect, it } from "vitest";
import { dayLabels, formatTotal, nextAction, shortHash, stepStatus } from "./settlement-model";

describe("settlement-model", () => {
  it("marks earlier steps done, the stage's step current and later steps pending __MCN_705_AC1", () => {
    expect([0, 1, 2, 3].map((i) => stepStatus("TOTALS_EXCHANGED", i))).toEqual(["done", "done", "current", "pending"]);
    expect([0, 1, 2, 3].map((i) => stepStatus("OPEN", i))).toEqual(["current", "pending", "pending", "pending"]);
    expect([0, 1, 2, 3].map((i) => stepStatus("FILE_GENERATED", i))).toEqual(["done", "done", "done", "done"]);
  });

  it("offers the next step's action, waits for the 0500 after cutover and stops once the file exists __MCN_705_AC1", () => {
    expect(nextAction("OPEN")).toBe("cutover");
    expect(nextAction("CUTOVER_DONE")).toBe("waitTotals");
    expect(nextAction("TOTALS_EXCHANGED")).toBe("reconcile");
    expect(nextAction("RECONCILED")).toBe("generate");
    expect(nextAction("FILE_GENERATED")).toBeNull();
  });

  it("derives the day, the next day and DE 15's MMDD from the business date, across a month end", () => {
    expect(dayLabels("2026-09-21")).toEqual({ day: "21/09", nextDay: "22/09", nextMmdd: "0922" });
    expect(dayLabels("2026-09-30")).toEqual({ day: "30/09", nextDay: "01/10", nextMmdd: "1001" });
  });

  it("formats counts as numbers and amounts as money in minor units", () => {
    expect(formatTotal({ metric: "DEBITS_COUNT", isoField: "76", acquirer: 1282, issuer: 1281, matches: false }, "acquirer", "704")).toBe("1.282");
    expect(formatTotal({ metric: "DEBITS_AMOUNT", isoField: "88", acquirer: 237184500, issuer: 1, matches: false }, "acquirer", "704")).toMatch(/^237\.184\.500\s₫$/);
  });

  it("shortens a SHA-256 to its first six and last four hex digits", () => {
    expect(shortHash(`a91f03${"0".repeat(54)}7c2e`)).toBe("a91f03…7c2e");
  });
});
