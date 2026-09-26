import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { MoneyVerificationPanel } from "@/components/chaos/MoneyVerificationPanel";
import type { ChaosRun } from "@/shared/api/chaos-client";

const passed: ChaosRun = {
  runId: "run-7",
  status: "PASSED",
  requested: 100,
  completed: 100,
  approved: 86,
  declined: 6,
  reversed: 8,
  openingBalanceTotal: 500_000_000,
  closingBalanceTotal: 484_090_000,
  ledgerDiscrepancy: 0,
};

const text = (el: Element | null) => el?.textContent?.replace(/\s/g, " ");

describe("MoneyVerificationPanel", () => {
  it("shows the five canvas rows with dashes and a not-run badge before any run __MCN_405_AC2", () => {
    renderWithIntl(<MoneyVerificationPanel run={undefined} expert={false} />);
    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "none");
    expect(screen.getByText("Chưa chạy thử")).toBeInTheDocument();
    expect(screen.getByText("Tổng số dư đầu ngày")).toBeInTheDocument();
    expect(screen.getByText("Giao dịch được duyệt")).toBeInTheDocument();
    expect(screen.queryByText(/— giao dịch/)).not.toBeInTheDocument();
    expect(screen.getAllByText("—")).toHaveLength(5);
  });

  it("flashes the zero discrepancy and says the books match on PASSED __MCN_405_AC2", () => {
    renderWithIntl(<MoneyVerificationPanel run={passed} expert={false} />);
    const panel = screen.getByTestId("money-verification");
    expect(panel).toHaveAttribute("data-result", "ok");
    expect(screen.getByText("Sổ sách khớp")).toBeInTheDocument();
    expect(screen.getByText("86 giao dịch được duyệt")).toBeInTheDocument();
    expect(text(screen.getByTestId("ledger-approved-value"))).toBe("−15.910.000 ₫");
    expect(text(screen.getByTestId("ledger-reversed-value"))).toBe("8 lệnh");
    expect(text(screen.getByTestId("ledger-current-value"))).toBe("484.090.000 ₫");
    const zero = screen.getByTestId("ledger-discrepancy-value");
    expect(text(zero)).toBe("0 ₫");
    expect(zero).toHaveAttribute("data-tone", "ok");
    expect(zero).toHaveAttribute("data-flash", "true");
  });

  it("drops the count from the expert approved label before any run __MCN_405_AC2", () => {
    renderWithIntl(<MoneyVerificationPanel run={undefined} expert />);
    expect(screen.getByText("Giao dịch RC 00")).toBeInTheDocument();
  });

  it("uses the expert labels in expert mode __MCN_405_AC2", () => {
    renderWithIntl(<MoneyVerificationPanel run={passed} expert />);
    expect(screen.getByText("Số dư đầu ngày (opening)")).toBeInTheDocument();
    expect(screen.getByText("Giao dịch RC 00 · 86 lệnh")).toBeInTheDocument();
    expect(screen.getByText("Σ ledger − Σ tran_log")).toBeInTheDocument();
  });

  it("shows a non-zero discrepancy in red with the run id __MCN_405_AC2", () => {
    renderWithIntl(<MoneyVerificationPanel run={{ ...passed, status: "FAILED", ledgerDiscrepancy: 500 }} expert={false} />);
    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "discrepancy");
    expect(screen.getByTestId("ledger-discrepancy-value")).toHaveAttribute("data-tone", "bad");
    expect(screen.getByText("Lệch trong lần chạy run-7")).toBeInTheDocument();
    expect(screen.getByText("Sổ sách lệch")).toBeInTheDocument();
  });

  it("shows pending money and no flash while the run is in flight __MCN_405_AC2", () => {
    renderWithIntl(<MoneyVerificationPanel run={{ ...passed, status: "RUNNING", closingBalanceTotal: 0 }} expert={false} />);
    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "pending");
    expect(screen.getByText("Đang kiểm chứng…")).toBeInTheDocument();
    expect(screen.getByTestId("ledger-discrepancy-value")).not.toHaveAttribute("data-flash");
  });

  it("shows a run that could not finish as an error with the provider's reason, not a ledger mismatch __CHA_G3", () => {
    const failed: ChaosRun = { ...passed, status: "FAILED", failureKind: "RUN_ERROR", failureDetail: "SAF not drained after the run (2 pending)", closingBalanceTotal: 0 };
    renderWithIntl(<MoneyVerificationPanel run={failed} expert={false} />);

    expect(screen.getByTestId("money-verification")).toHaveAttribute("data-result", "error");
    expect(screen.getByText("Lần chạy thử bị lỗi")).toBeInTheDocument();
    expect(screen.queryByText("Sổ sách lệch")).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("SAF not drained after the run (2 pending)");
  });
});
