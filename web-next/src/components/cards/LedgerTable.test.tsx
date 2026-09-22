import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { LedgerTable } from "./LedgerTable";

describe("LedgerTable", () => {
  it("renders one row per journal entry with balanced postings", () => {
    render(
      <NextIntlClientProvider locale="en" messages={en}>
        <LedgerTable
          entries={[
            {
              journalId: "j1",
              occurredAt: new Date().toISOString(),
              description: "Purchase",
              entryType: "PURCHASE",
              rrn: "x",
              postings: [
                { account: "970436******4417", direction: "DEBIT", amount: { amount: 10000, currency: "704" } },
                { account: "SETTLEMENT_SUSPENSE", direction: "CREDIT", amount: { amount: 10000, currency: "704" } },
              ],
            },
          ]}
        />
      </NextIntlClientProvider>,
    );
    expect(screen.getByText(/purchase/i)).toBeInTheDocument();
    expect(screen.getAllByText(/970436|SETTLEMENT_SUSPENSE/)).toHaveLength(2);
  });
});
