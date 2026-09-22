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
    expect(screen.getByText(/-10,000 VND/)).toBeInTheDocument();
  });

  it("renders a positive delta with a leading plus sign", () => {
    renderWithIntl(
      <MoneyPanel money={[{ label: "Refund", delta: 5000, balanceAfter: 990000, atStep: 2 }]} currency="704" />,
    );
    expect(screen.getByText(/\+5,000 VND/)).toBeInTheDocument();
    expect(screen.getByText(/990,000 VND/)).toBeInTheDocument();
  });
});
