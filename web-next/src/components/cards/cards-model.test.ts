import { describe, expect, it } from "vitest";
import type { JournalEntry } from "@/shared/api/cards-client";
import { activeHoldTotal, cardTag, ledgerRow, statusKind, usagePercent } from "./cards-model";

const vnd = (amount: number) => ({ amount, currency: "704" });
const TODAY = new Date(2026, 8, 21);

function entry(overrides: Partial<JournalEntry>): JournalEntry {
  return { journalId: "1", occurredAt: "2026-09-21T07:32:00Z", description: "Mua hàng · Cà phê Góc Phố", entryType: "PURCHASE", rrn: null, postings: [], ...overrides };
}

describe("statusKind (MCN-309-AC1 badge per card)", () => {
  it("is active for an ACTIVE card inside its expiry month", () => {
    expect(statusKind({ status: "ACTIVE", expiry: "09/26" }, TODAY)).toBe("active");
  });

  it("is expired once the expiry month is over, even while the issuer still says ACTIVE", () => {
    expect(statusKind({ status: "ACTIVE", expiry: "08/26" }, TODAY)).toBe("expired");
    expect(statusKind({ status: "EXPIRED", expiry: "12/29" }, TODAY)).toBe("expired");
  });

  it("is locked for any non-active issuer status", () => {
    expect(statusKind({ status: "BLOCKED", expiry: "07/27" }, TODAY)).toBe("locked");
    expect(statusKind({ status: "LOST", expiry: "07/27" }, TODAY)).toBe("locked");
  });
});

describe("balances and limits", () => {
  it("sums only ACTIVE holds", () => {
    const hold = (amount: number, status: "ACTIVE" | "RELEASED") => ({ holdId: String(amount), merchantName: "x", amount: vnd(amount), expiresAt: "2026-09-26T00:00:00Z", status });
    expect(activeHoldTotal([hold(1_500_000, "ACTIVE"), hold(90_000, "RELEASED")])).toBe(1_500_000);
  });

  it("rounds usage to a whole percent, caps it at 100 and treats an unset limit as 0%", () => {
    expect(usagePercent(915_000, 10_000_000)).toBe(9);
    expect(usagePercent(986_500, 500_000)).toBe(100);
    expect(usagePercent(2_018_000, 0)).toBe(0);
  });

  it("maps a masked PAN to its fixture colour tag", () => {
    expect(cardTag("970436******4417")).toBe("normal");
    expect(cardTag("970436******0000")).toBeUndefined();
  });
});

describe("ledgerRow (MCN-309-AC1 ledger table)", () => {
  it("reads a purchase as money out with its debit and credit legs", () => {
    const row = ledgerRow(
      entry({
        postings: [
          { account: "ACC-crd_normal0001", direction: "DEBIT", amount: vnd(250_000) },
          { account: "SETTLEMENT_SUSPENSE", direction: "CREDIT", amount: vnd(250_000) },
        ],
      }),
    );
    expect(row).toMatchObject({ effect: -250_000, debitAccount: "ACC-crd_normal0001", creditAccount: "SETTLEMENT_SUSPENSE", amount: 250_000, description: "Mua hàng · Cà phê Góc Phố" });
  });

  it("reads a credit to the customer as money in", () => {
    const row = ledgerRow(
      entry({
        postings: [
          { account: "INCOMING_TRANSFER", direction: "DEBIT", amount: vnd(15_000_000) },
          { account: "ACC-crd_normal0001", direction: "CREDIT", amount: vnd(15_000_000) },
        ],
      }),
    );
    expect(row.effect).toBe(15_000_000);
  });

  it("keeps a journal without postings as an unposted row", () => {
    expect(ledgerRow(entry({ postings: [] }))).toMatchObject({ effect: 0, debitAccount: null, creditAccount: null, amount: 0 });
  });

  it("drops the issuer's generated description so the screen can name the entry type instead", () => {
    expect(ledgerRow(entry({ description: "PURCHASE journal 244" })).description).toBeNull();
  });
});
