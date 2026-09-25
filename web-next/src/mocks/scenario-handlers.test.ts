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
