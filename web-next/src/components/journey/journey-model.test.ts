import { describe, expect, it } from "vitest";
import type { Journey } from "@/shared/api/journey-client";
import type { JournalEntry } from "@/shared/api/cards-client";
import { customerBalances, formatOffset, moneyRows, outcomeOf } from "./journey-model";

type Step = Journey["steps"][number];
const step = (seq: number, code: Step["code"], kind: Step["kind"] = "OK"): Step => ({
  seq, code, kind, actor: "POS", offsetMs: seq * 10, title: "", easyText: "", technicalText: "", message: null,
});

function entry(journalId: string, rrn: string | null, direction: "DEBIT" | "CREDIT", amount: number): JournalEntry {
  return {
    journalId, rrn, occurredAt: "2026-09-25T06:00:00Z", description: "", entryType: direction === "DEBIT" ? "PURCHASE" : "REVERSAL",
    postings: [
      { account: "ACC-crd_second0006", direction, amount: { amount, currency: "704" } },
      { account: "SETTLEMENT_SUSPENSE", direction: direction === "DEBIT" ? "CREDIT" : "DEBIT", amount: { amount, currency: "704" } },
    ],
  };
}

describe("outcomeOf", () => {
  it("reads a reversal after a timeout as an automatic cancellation", () => {
    expect(outcomeOf("REVERSED", [step(1, "REQUEST_SENT"), step(2, "NO_RESPONSE", "WARN")])).toBe("autoReversed");
  });
  it("reads a reversal without a timeout as a cancellation", () => {
    expect(outcomeOf("REVERSED", [step(1, "ISSUER_APPROVED"), step(2, "REVERSAL_CONFIRMED", "REVERSAL")])).toBe("cancelled");
  });
  it.each([
    ["APPROVED", "approved"],
    ["DECLINED", "declined"],
    ["REVERSAL_PENDING", "reversing"],
    ["TIMED_OUT", "reversing"],
    ["SENT", "pending"],
  ] as const)("maps %s to %s", (status, outcome) => {
    expect(outcomeOf(status, [])).toBe(outcome);
  });
});

describe("formatOffset (canvas: '+182 ms', '+30,09 s')", () => {
  it("keeps milliseconds under a second", () => expect(formatOffset(182, "vi")).toBe("+182 ms"));
  it("switches to seconds with two decimals in the locale's separator", () => {
    expect(formatOffset(30_090, "vi")).toBe("+30,09 s");
    expect(formatOffset(30_090, "en")).toBe("+30.09 s");
  });
});

describe("customerBalances", () => {
  const ledger = [
    entry("5", "later", "DEBIT", 1_000), // after our transaction, newest first
    entry("4", "ours", "CREDIT", 600_000),
    entry("3", "other", "DEBIT", 2_000),
    entry("2", "ours", "DEBIT", 600_000),
    entry("1", "older", "DEBIT", 5_000),
  ];

  it("rewinds the current balance to just before the transaction's first journal", () => {
    // now 4_997_000: before = now + 1_000 - 600_000 + 2_000 + 600_000
    expect(customerBalances(ledger, 4_997_000, "ours")).toEqual({ before: 5_000_000, final: 5_000_000, own: [-600_000, 600_000] });
  });

  it("is null when the issuer has no journal for the transaction (a decline)", () => {
    expect(customerBalances(ledger, 4_997_000, "declined")).toBeNull();
  });
});

describe("moneyRows", () => {
  // A timed-out purchase: the issuer debited it, but the acquirer never saw the approval.
  const journey = {
    transaction: { status: "REVERSED" },
    steps: [step(1, "POS_REQUEST"), step(2, "REQUEST_SENT"), step(3, "NO_RESPONSE", "WARN"), step(4, "REVERSAL_CONFIRMED", "REVERSAL")],
    money: [{ label: "Refund", delta: 600_000, balanceAfter: null, atStep: 4 }],
  } as unknown as Parameters<typeof moneyRows>[0];

  it("places the issuer's real journals at their steps, framed by the balance before and after (MCN-406-AC1)", () => {
    expect(moneyRows(journey, { before: 5_000_000, final: 5_000_000, own: [-600_000, 600_000] })).toEqual([
      { kind: "before", amount: 5_000_000, atIndex: 0 },
      { kind: "debit", amount: -600_000, atIndex: 1 },
      { kind: "credit", amount: 600_000, atIndex: 3 },
      { kind: "final", amount: 5_000_000, atIndex: 3 },
    ]);
  });

  it("falls back to the provider's deltas when the ledger is unavailable", () => {
    expect(moneyRows(journey, null)).toEqual([{ kind: "credit", amount: 600_000, atIndex: 3 }]);
  });

  it("shows no money movement for a decline", () => {
    const declined = { ...journey, transaction: { status: "DECLINED" } } as typeof journey;
    expect(moneyRows(declined, null)).toEqual([]);
  });
});
