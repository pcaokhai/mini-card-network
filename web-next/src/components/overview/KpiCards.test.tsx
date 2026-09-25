import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { KpiCards } from "./KpiCards";
import { useDisplayMode } from "@/shared/state/display-mode";
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

  it("Easy mode compares today's volume with yesterday", () => {
    useDisplayMode.setState({ mode: "easy" });
    renderWithIntl(<KpiCards overview={{ ...base, transactionsDeltaPct: 0.12 }} />);
    expect(screen.getByText("Tăng 12% so với hôm qua")).toBeInTheDocument();
  });

  it("MCN-306-AC3: Expert mode pairs p99 with the p50 when the gateway sends it", () => {
    useDisplayMode.setState({ mode: "expert" });
    renderWithIntl(<KpiCards overview={{ ...base, p50LatencyMs: 96 }} />);
    expect(screen.getByText("p99 end-to-end · p50 96 ms")).toBeInTheDocument();
    useDisplayMode.setState({ mode: "easy" });
  });

  it("counts KPI values up while exposing only the settled value to screen readers", () => {
    renderWithIntl(<KpiCards overview={base} />);
    expect(screen.getByText("128")).toHaveClass("sr-only");
  });

  it("shows the peak rate rounded for reading, not as a raw float", () => {
    useDisplayMode.setState({ mode: "expert" });
    renderWithIntl(
      <KpiCards overview={{ ...base, throughput: [{ at: "2026-09-21T05:06:00Z", tps: 0.04666666666666667 }] }} />,
    );
    expect(screen.getByText(/TPS đỉnh 0,05 lúc/)).toBeInTheDocument();
    useDisplayMode.setState({ mode: "easy" });
  });
});
