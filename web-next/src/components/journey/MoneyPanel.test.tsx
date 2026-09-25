import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { MoneyPanel } from "./MoneyPanel";

describe("MoneyPanel", () => {
  it("renders each money row with a signed delta", () => {
    renderWithIntl(
      <MoneyPanel
        money={[{ label: "Purchase", delta: -10000, balanceAfter: null, atStep: 1 }]}
        currency="704"
      />,
    );
    expect(screen.getByText("Purchase")).toBeInTheDocument();
    expect(screen.getByText(/-10\.000\s₫/)).toBeInTheDocument();
  });

  it("renders a positive delta with a leading plus sign", () => {
    renderWithIntl(
      <MoneyPanel money={[{ label: "Refund", delta: 5000, balanceAfter: 990000, atStep: 2 }]} currency="704" />,
    );
    expect(screen.getByText(/\+5\.000\s₫/)).toBeInTheDocument();
    expect(screen.getByText(/990\.000\s₫/)).toBeInTheDocument();
  });

  it("shows a debit then a refund row with distinct sign styling (MCN-406-AC1)", () => {
    renderWithIntl(
      <MoneyPanel
        money={[
          { label: "Purchase", delta: -10000, balanceAfter: null, atStep: 2 },
          { label: "Refund", delta: 10000, balanceAfter: null, atStep: 3 },
        ]}
        currency="704"
      />,
    );
    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveAttribute("data-sign", "debit");
    expect(rows[1]).toHaveAttribute("data-sign", "credit");
  });
});
