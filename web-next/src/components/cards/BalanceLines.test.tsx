import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { BalanceLines } from "./BalanceLines";

describe("BalanceLines", () => {
  it("renders ledger, available, and held amounts as three distinct lines", () => {
    render(
      <NextIntlClientProvider locale="en" messages={en}>
        <BalanceLines
          ledgerBalance={{ amount: 500000, currency: "704" }}
          availableBalance={{ amount: 480000, currency: "704" }}
          holds={[{ holdId: "h1", merchantName: "x", amount: { amount: 20000, currency: "704" }, expiresAt: new Date().toISOString(), status: "ACTIVE" }]}
        />
      </NextIntlClientProvider>,
    );
    expect(screen.getByText(/ledger/i)).toBeInTheDocument();
    expect(screen.getByText(/available/i)).toBeInTheDocument();
    expect(screen.getByText(/held/i)).toBeInTheDocument();
  });
});
