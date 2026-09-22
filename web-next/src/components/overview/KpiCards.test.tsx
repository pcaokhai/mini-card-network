import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { KpiCards } from "./KpiCards";
import type { Overview } from "@/shared/api/overview-client";

const base: Overview = {
  transactionsToday: 128,
  approvalRate: 0.92,
  p99LatencyMs: 84,
  ledgerMatches: true,
  throughput: [],
  declineReasons: [],
};

describe("KpiCards", () => {
  it("renders the 4 required KPIs", () => {
    renderWithIntl(<KpiCards overview={base} />);
    expect(screen.getByText("128")).toBeInTheDocument();
    expect(screen.getByText(/92%/)).toBeInTheDocument();
    expect(screen.getByText(/84\s*ms/)).toBeInTheDocument();
    expect(screen.getByTestId("kpi-ledger")).toBeInTheDocument();
  });

  it("shows a warning state when ledgerMatches is false", () => {
    renderWithIntl(<KpiCards overview={{ ...base, transactionsToday: 1, approvalRate: 1, p99LatencyMs: 1, ledgerMatches: false }} />);
    expect(screen.getByTestId("kpi-ledger")).toHaveAttribute("data-status", "warn");
  });
});
