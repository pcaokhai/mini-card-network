import { setupServer } from "msw/node";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { scenarioHandlers } from "./scenario-handlers";
import type { components } from "@/shared/api/generated/schema";

type TransactionSummary = components["schemas"]["TransactionSummary"];

// What each fixture card (contracts/fixtures/cards.json) can produce on the real stack, by last 4.
const CARD_OUTCOMES: Record<string, (row: TransactionSummary) => boolean> = {
  "4417": () => true, // normal card: any outcome a purchase can have
  "5540": () => true,
  "9021": (row) => row.status !== "APPROVED" || row.amount.amount <= 80_000, // 80 000 ₫ balance
  "1208": (row) => row.status !== "APPROVED" || row.amount.amount <= 500_000, // per-transaction limit
  "3310": (row) => row.status === "DECLINED" && row.responseCode === "62", // blocked
  "7765": (row) => row.status === "DECLINED" && row.responseCode === "54", // expired
};

const server = setupServer(...scenarioHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterAll(() => server.close());

describe("dev:mock scenario transactions", () => {
  it("MCN-002: only show outcomes the fixture cards can produce, and never a full PAN", async () => {
    const res = await fetch("http://localhost/v1/transactions");
    const { items } = (await res.json()) as { items: TransactionSummary[] };

    expect(items.length).toBeGreaterThan(0);
    for (const row of items) {
      expect(row.maskedPan).toMatch(/^970436\*{6}\d{4}$/);
      const canProduce = CARD_OUTCOMES[row.maskedPan.slice(-4)];
      expect(canProduce, `unknown card ${row.maskedPan}`).toBeDefined();
      expect(canProduce(row), `${row.merchantName} ${row.maskedPan} ${row.status}`).toBe(true);
    }
  });
});

describe("dev:mock POS outcomes", () => {
  async function purchase(cardToken: string, amount: number, idempotencyKey = crypto.randomUUID()) {
    const res = await fetch("http://localhost/v1/transactions/purchases", {
      method: "POST",
      headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({ terminalId: "00000042", cardToken, entryMode: "CHIP_NO_PIN", amount: { amount, currency: "704" } }),
    });
    return (await res.json()) as components["schemas"]["Transaction"];
  }
  it("MCN-305: each fixture card declines the way the real issuer does", async () => {
    expect((await purchase("tok_low", 350_000)).responseCode).toBe("51");
    expect((await purchase("tok_blocked", 90_000)).responseCode).toBe("62");
    expect((await purchase("tok_expired", 150_000)).responseCode).toBe("54");
    expect((await purchase("tok_limit", 600_000)).responseCode).toBe("61");
  });

  it("MCN-305: a normal card approves", async () => {
    const tx = await purchase("tok_normal", 250_000);
    expect(tx).toMatchObject({ status: "APPROVED", responseCode: "00", maskedPan: "970436******4417" });
  });

  it("replays a repeated Idempotency-Key instead of charging twice", async () => {
    const first = await purchase("tok_low", 50_000, "same-key");
    const again = await purchase("tok_low", 50_000, "same-key");
    expect(again.rrn).toBe(first.rrn);
    // tok_low holds 80 000 ₫: a second real charge of 50 000 would have declined 51.
    expect((await purchase("tok_low", 50_000)).responseCode).toBe("51");
  });

  it("answers a completion of an unknown RRN like the gateway: 404 unknown-transaction __POS_G13", async () => {
    const res = await fetch("http://localhost/v1/transactions/626807999999/completions", {
      method: "POST",
      headers: { "Content-Type": "application/json", "Idempotency-Key": "completion-unknown" },
      body: JSON.stringify({ amount: { amount: 10_000, currency: "704" } }),
    });

    expect(res.status).toBe(404);
    expect(res.headers.get("Content-Type")).toContain("application/problem+json");
    expect(await res.json()).toMatchObject({ type: "unknown-transaction", status: 404 });
  });
});
