import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { HoldsList } from "./HoldsList";

describe("HoldsList", () => {
  it("renders one row per active hold showing merchant, amount, and expiry", () => {
    render(
      <NextIntlClientProvider locale="en" messages={en}>
        <HoldsList
          holds={[
            { holdId: "h1", merchantName: "Coffee Shop", amount: { amount: 20000, currency: "704" }, expiresAt: new Date().toISOString(), status: "ACTIVE" },
            { holdId: "h2", merchantName: "Old Hold", amount: { amount: 5000, currency: "704" }, expiresAt: new Date().toISOString(), status: "RELEASED" },
          ]}
        />
      </NextIntlClientProvider>,
    );
    expect(screen.getByText("Coffee Shop")).toBeInTheDocument();
    expect(screen.queryByText("Old Hold")).not.toBeInTheDocument();
    expect(screen.getAllByRole("row")).toHaveLength(1);
  });
});
