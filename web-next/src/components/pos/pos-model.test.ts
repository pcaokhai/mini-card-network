import { describe, expect, it } from "vitest";
import type { Transaction } from "@/shared/api/pos-client";
import { DISPLAY_CARDS, ENTRY_MODES, kindOf, nextAmount, outcomeOf, techLine } from "./pos-model";

const approved: Transaction = {
  rrn: "626807000301",
  stan: "000301",
  type: "PURCHASE",
  status: "APPROVED",
  responseCode: "00",
  authCode: "A00301",
  amount: { amount: 250_000, currency: "704" },
  maskedPan: "970436******4417",
  terminalId: "00000042",
  merchantName: "Cà phê Góc Phố",
  createdAt: "2026-09-25T07:00:00Z",
};

describe("outcomeOf", () => {
  it("MCN-305-AC3 maps approvals and the canvas decline codes", () => {
    expect(outcomeOf(approved)).toBe("approved");
    for (const rc of ["51", "54", "61", "62", "91"] as const) {
      expect(outcomeOf({ ...approved, status: "DECLINED", responseCode: rc })).toBe(`rc${rc}`);
    }
    expect(outcomeOf({ ...approved, status: "DECLINED", responseCode: "30" })).toBe("declined");
    expect(outcomeOf({ ...approved, status: "FAILED", responseCode: null })).toBe("declined");
  });

  it("MCN-305-AC5 maps a timeout or queued reversal to the rev kind", () => {
    expect(outcomeOf({ ...approved, status: "TIMED_OUT", responseCode: null })).toBe("reversalPending");
    expect(outcomeOf({ ...approved, status: "REVERSAL_PENDING", responseCode: null })).toBe("reversalPending");
    expect(outcomeOf({ ...approved, status: "REVERSED", responseCode: null })).toBe("reversed");
    expect(kindOf("reversed")).toBe("rev");
    expect(kindOf("rc51")).toBe("bad");
    expect(kindOf("approved")).toBe("ok");
  });
});

describe("techLine", () => {
  it("renders the canvas MTI/STAN/RC line with DE 38 for an approved purchase", () => {
    expect(techLine(approved)).toBe("0200 STAN 000301 → 0210 · RC 00 · field 38 = A00301");
  });

  it("uses 0100/0110 for a pre-authorization and omits a missing auth code", () => {
    expect(techLine({ ...approved, type: "PREAUTH", status: "DECLINED", responseCode: "62", authCode: null })).toBe(
      "0100 STAN 000301 → 0110 · RC 62",
    );
  });

  it("shows the timeout and the 0420 reversal for the rev kind", () => {
    expect(techLine({ ...approved, status: "REVERSED", responseCode: null })).toBe(
      "0200 STAN 000301 → timeout 30s → 0420 (field 90 = 0200000301…) → 0430",
    );
    expect(techLine({ ...approved, status: "TIMED_OUT", responseCode: null })).toBe(
      "0200 STAN 000301 → timeout 30s → 0420 (field 90 = 0200000301…) → SAF",
    );
  });
});

describe("nextAmount", () => {
  it("MCN-305-AC1 follows the canvas keypad rules", () => {
    expect(nextAmount("0", "5")).toBe("5");
    expect(nextAmount("25", "0")).toBe("250");
    expect(nextAmount("250", "⌫")).toBe("25");
    expect(nextAmount("2", "⌫")).toBe("0");
    expect(nextAmount("250", "C")).toBe("0");
    expect(nextAmount("1234567890", "1")).toBe("1234567890");
  });
});

describe("display data", () => {
  it("never carries a PAN, only last 4 and non-sensitive references", () => {
    expect(DISPLAY_CARDS).toHaveLength(6);
    expect(JSON.stringify(DISPLAY_CARDS)).not.toMatch(/\d{12,}/);
  });

  it("offers only the contract's no-PIN entry modes with the gateway's DE 22", () => {
    expect(ENTRY_MODES.map((e) => [e.entryMode, e.de22])).toEqual([
      ["CHIP_NO_PIN", "052"],
      ["MANUAL_NO_PIN", "012"],
    ]);
  });
});
