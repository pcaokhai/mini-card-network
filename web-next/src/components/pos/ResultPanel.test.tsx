import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import type { Transaction } from "@/shared/api/pos-client";
import { ResultPanel } from "./ResultPanel";
import type { PosResult } from "./result-view";

const approved: Transaction = {
  rrn: "626807000301",
  stan: "000301",
  type: "PURCHASE",
  status: "APPROVED",
  responseCode: "00",
  authCode: "A00301",
  amount: { amount: 250_000, currency: "704" },
  maskedPan: "970436******4417",
  terminalId: "00000042",
  merchantName: "Cà phê Góc Phố",
  createdAt: "2026-09-25T07:00:00Z",
};

const txResult = (tx: Transaction, balance: number | null = 4_750_000): PosResult => ({
  kind: "tx",
  tx,
  last4: "4417",
  entry: "chip",
  balance,
});

describe("ResultPanel", () => {
  it("shows the canvas empty state before the first transaction", () => {
    renderWithIntl(<ResultPanel processing={false} result={null} expert={false} />);
    expect(screen.getByText("Thực hiện giao dịch đầu tiên")).toBeInTheDocument();
  });

  it("MCN-305-AC2 shows the spinner state, with the MUX wait line in Expert mode", () => {
    renderWithIntl(<ResultPanel processing result={null} expert />);
    expect(screen.getByText("Đang gửi tới ngân hàng phát hành…")).toBeInTheDocument();
    expect(screen.getByText("0200 đã gửi · MUX chờ 0210 (timeout 30 s)")).toBeInTheDocument();
  });

  it("MCN-305-AC3 approved: canvas copy and steps, no tech line in Easy mode", () => {
    const { container } = renderWithIntl(<ResultPanel processing={false} result={txResult(approved)} expert={false} />);
    expect(screen.getByText("Thanh toán thành công")).toBeInTheDocument();
    expect(screen.getByText("Đã trừ 250.000 ₫. Số dư còn 4.750.000 ₫.")).toBeInTheDocument();
    expect(screen.queryByText(/STAN/)).not.toBeInTheDocument();
    expect(within(screen.getByRole("list")).getAllByRole("listitem")).toHaveLength(4);
    expect(screen.getByText("Máy in hóa đơn, khách nhận hàng.")).toBeInTheDocument();
    expect(container.querySelector('[data-kind="ok"]')).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem hành trình chi tiết" })).toHaveAttribute(
      "href",
      "/transactions/626807000301",
    );
  });

  it("MCN-305-AC3 Expert mode adds the MTI/STAN/RC line", () => {
    renderWithIntl(<ResultPanel processing={false} result={txResult(approved)} expert />);
    expect(screen.getByText("0200 STAN 000301 → 0210 · RC 00 · field 38 = A00301")).toBeInTheDocument();
  });

  it("MCN-305-AC3 a decline uses the bad kind so the icon shakes", () => {
    const declined = { ...approved, status: "DECLINED", responseCode: "62", authCode: null } as const;
    const { container } = renderWithIntl(<ResultPanel processing={false} result={txResult(declined)} expert={false} />);
    expect(screen.getByText("Thẻ đang bị khóa")).toBeInTheDocument();
    expect(screen.getByText("Trả kết quả từ chối")).toBeInTheDocument();
    expect(container.querySelector('[data-kind="bad"] .pos-result__icon')).toBeInTheDocument();
  });

  it("MCN-305-AC5 a reversed transaction shows the automatic reversal", () => {
    const reversed = { ...approved, status: "REVERSED", responseCode: null, authCode: null } as const;
    const { container } = renderWithIntl(<ResultPanel processing={false} result={txResult(reversed)} expert={false} />);
    expect(screen.getByText("Giao dịch đã được tự động hủy")).toBeInTheDocument();
    expect(screen.getByText("Tự động hủy giao dịch")).toBeInTheDocument();
    expect(container.querySelector('[data-kind="rev"]')).toBeInTheDocument();
  });

  it("uses the balance from the balance inquiry response", () => {
    const balance: Transaction = { ...approved, type: "BALANCE", balance: { amount: 1_500_000, currency: "704" } };
    renderWithIntl(<ResultPanel processing={false} result={txResult(balance, null)} expert={false} />);
    expect(screen.getByText("Số dư khả dụng là 1.500.000 ₫.")).toBeInTheDocument();
  });
});
