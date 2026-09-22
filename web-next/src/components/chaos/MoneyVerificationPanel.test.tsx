import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { MoneyVerificationPanel } from "@/components/chaos/MoneyVerificationPanel";

describe("MoneyVerificationPanel", () => {
  it("shows a zero-discrepancy flash on PASSED __MCN_405_AC2", () => {
    renderWithIntl(
      <MoneyVerificationPanel
        run={{
          runId: "r1",
          status: "PASSED",
          requested: 10,
          completed: 10,
          approved: 8,
          declined: 1,
          reversed: 1,
          openingBalanceTotal: 1_000_000,
          closingBalanceTotal: 1_000_000,
          ledgerDiscrepancy: 0,
        }}
      />,
    );
    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "ok");
  });

  it("shows the run id in red on a non-zero discrepancy __MCN_405_AC2", () => {
    renderWithIntl(
      <MoneyVerificationPanel
        run={{
          runId: "r2",
          status: "FAILED",
          requested: 10,
          completed: 10,
          approved: 9,
          declined: 0,
          reversed: 1,
          openingBalanceTotal: 1_000_000,
          closingBalanceTotal: 999_500,
          ledgerDiscrepancy: 500,
        }}
      />,
    );
    const panel = screen.getByTestId("money-verification");
    expect(panel).toHaveAttribute("data-result", "discrepancy");
    expect(panel).toHaveTextContent("r2");
  });

  it("shows a RUNNING progress state with no result flash yet __MCN_405_AC2", () => {
    renderWithIntl(
      <MoneyVerificationPanel
        run={{
          runId: "r3",
          status: "RUNNING",
          requested: 10,
          completed: 4,
          approved: 3,
          declined: 1,
          reversed: 0,
          openingBalanceTotal: 1_000_000,
          closingBalanceTotal: 0,
          ledgerDiscrepancy: 0,
        }}
      />,
    );
    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "pending");
  });
});
