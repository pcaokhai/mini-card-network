import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { HoldsList, type Hold } from "./HoldsList";

function renderHolds(holds: Hold[]) {
  return render(
    <NextIntlClientProvider locale="en" messages={en}>
      <HoldsList holds={holds} />
    </NextIntlClientProvider>,
  );
}

describe("HoldsList", () => {
  it("renders one row per hold of every status, not only ACTIVE", () => {
    renderHolds([
      { holdId: "h1", merchantName: "Coffee Shop", amount: { amount: 20000, currency: "704" }, expiresAt: new Date().toISOString(), status: "ACTIVE" },
      { holdId: "h2", merchantName: "Old Hold", amount: { amount: 5000, currency: "704" }, expiresAt: new Date().toISOString(), status: "RELEASED" },
    ]);
    expect(screen.getByText("Coffee Shop")).toBeInTheDocument();
    expect(screen.getByText("Old Hold")).toBeInTheDocument();
    expect(screen.getAllByRole("row")).toHaveLength(2);
  });

  it("shows a status pill per hold reflecting its completion state", () => {
    renderHolds([
      { holdId: "h2", merchantName: "Hotel", amount: { amount: 20000, currency: "704" }, expiresAt: "2026-09-20T00:00:00Z", status: "COMPLETED" },
    ]);
    expect(screen.getByText(/completed/i)).toBeInTheDocument();
  });

  it("shows the empty state when there are no holds at all", () => {
    renderHolds([]);
    expect(screen.getByText(/no holds yet/i)).toBeInTheDocument();
  });
});
