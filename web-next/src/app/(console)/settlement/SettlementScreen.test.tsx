import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { createSettlementHandlers } from "@/mocks/pages/settlement";
import { useDisplayMode } from "@/shared/state/display-mode";
import { renderWithIntl } from "@/test/render";
import { SettlementScreen } from "./SettlementScreen";

const server = setupServer();
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const steps = () => within(screen.getByRole("region", { name: "Tiến trình chốt ngày" })).getAllByRole("listitem");
const totals = () => screen.getByRole("region", { name: "Bảng tổng kết" });
const breaks = () => screen.getByRole("region", { name: "Chênh lệch" });
const file = () => screen.getByRole("region", { name: "File quyết toán" });

async function renderAt(stage: Parameters<typeof createSettlementHandlers>[0]) {
  server.use(...createSettlementHandlers(stage));
  renderWithIntl(<SettlementScreen businessDate="2026-09-21" />);
  await screen.findByText("Khóa sổ ngày 21/09");
}

async function resolveAll() {
  for (const name of ["Yêu cầu ngân hàng phát hành kiểm tra", "Khớp lại khi lệnh hủy tới nơi", "Điều chỉnh theo số tiền chốt"]) {
    await userEvent.click(await screen.findByRole("button", { name }));
  }
  await within(breaks()).findByText("Đã xử lý hết");
}

describe("SettlementScreen", () => {
  beforeEach(() => act(() => useDisplayMode.setState({ mode: "easy" })));

  it("drives the four steps from the day's stage __MCN_705_AC1", async () => {
    await renderAt("TOTALS_EXCHANGED");
    expect(steps().map((s) => s.getAttribute("data-status"))).toEqual(["done", "done", "current", "pending"]);
    expect(steps()[2]).toHaveAttribute("aria-current", "step");
    expect(within(steps()[2] as HTMLElement).getByText("Tiếp theo")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chạy bước 3: đối soát" })).toBeEnabled();
    expect(within(file()).getByText("File sẽ được tạo ở bước 4, sau khi mọi chênh lệch đã được xử lý.")).toBeInTheDocument();
  });

  it("shows totals with match badges from the API __MCN_705_AC1", async () => {
    await renderAt("TOTALS_EXCHANGED");
    const rows = within(totals()).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(5);
    expect(within(rows[0] as HTMLElement).getByText("Số giao dịch ghi nợ")).toBeInTheDocument();
    expect(within(rows[0] as HTMLElement).getByText("1.282")).toBeInTheDocument();
    expect(within(rows[0] as HTMLElement).getByText("1.281")).toBeInTheDocument();
    expect(within(totals()).getAllByText("Lệch")).toHaveLength(5);
    expect(within(breaks()).getByText(/Danh sách chênh lệch sẽ có sau bước 3/)).toBeInTheDocument();
  });

  it("waits for the 0500 totals after the cutover, then offers reconciliation __MCN_705_AC1", async () => {
    await renderAt("OPEN");
    expect(within(totals()).getByText(/Bảng tổng kết sẽ có sau bước 2/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Chạy bước 1: khóa sổ" }));
    expect(await screen.findByRole("button", { name: "Đang gửi tổng kết…" })).toBeDisabled();
    expect(await screen.findByRole("button", { name: "Chạy bước 3: đối soát" }, { timeout: 4000 })).toBeEnabled();
  });

  it("reconciles, then lists the breaks with resolve actions __MCN_705_AC1", async () => {
    await renderAt("TOTALS_EXCHANGED");
    await userEvent.click(screen.getByRole("button", { name: "Chạy bước 3: đối soát" }));
    await within(breaks()).findByText("3 chưa xử lý");
    expect(within(totals()).getByText("Khớp 1.279 / 1.282 giao dịch")).toBeInTheDocument();
    expect(within(breaks()).getByText("Thiếu ở ngân hàng phát hành")).toBeInTheDocument();
    expect(within(breaks()).getByText("Mã tra soát 626514000131")).toBeInTheDocument();
    expect(within(breaks()).getByText(/185\.000\s₫/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chạy bước 4: xuất file" })).toBeEnabled();
  });

  it("shows the API's open-breaks problem detail inline, shaking, when the file is asked for too early __MCN_705_AC2", async () => {
    await renderAt("RECONCILED");
    await userEvent.click(screen.getByRole("button", { name: "Chạy bước 4: xuất file" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Còn 3 chênh lệch chưa xử lý. Hãy xử lý hết trước khi xuất file quyết toán.");
    expect(alert).toHaveClass("set-shake");
  });

  it("resolves every break, turns the totals to match and produces the clearing file __MCN_705_AC1", async () => {
    await renderAt("RECONCILED");
    await resolveAll();
    expect(within(breaks()).getByText("Đã điều chỉnh")).toBeInTheDocument();
    expect(within(totals()).getAllByText("Khớp")).toHaveLength(5);
    expect(within(totals()).getByText("Khớp 1.282 / 1.282 giao dịch")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Chạy bước 4: xuất file" }));
    await within(file()).findByText("CLR_970499_20260921_001.csv");
    expect(within(file()).getByText(/^1\.282 dòng · 228\.164\.500\s₫$/)).toBeInTheDocument();
    expect(within(file()).getByText("a91f03…7c2e")).toBeInTheDocument();
    await waitFor(() => expect(steps().map((s) => s.getAttribute("data-status"))).toEqual(["done", "done", "done", "done"]));
    expect(screen.queryByRole("button", { name: /^Chạy bước/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("speaks the protocol in Expert mode __MCN_705_AC1", async () => {
    act(() => useDisplayMode.setState({ mode: "expert" }));
    await renderAt("RECONCILED");
    expect(screen.getByText("0800 · field 70 = 201 · field 15 = 0922")).toBeInTheDocument();
    expect(within(totals()).getByRole("heading", { name: "So sánh 0500 và số liệu issuer" })).toBeInTheDocument();
    expect(within(totals()).getByText("Acquirer (0500)")).toBeInTheDocument();
    expect(within(totals()).getByText("Debits, number · F76")).toBeInTheDocument();
    expect(await within(breaks()).findByText("MISSING_AT_ISSUER")).toBeInTheDocument();
    expect(within(breaks()).getByText("RRN 626514000131")).toBeInTheDocument();
    expect(screen.getByText("net_position · participant 970499 · 21/09")).toBeInTheDocument();
  });

  it("shows a designed unavailable state when the stack has no settlement service yet", async () => {
    server.use(http.get("*/v1/settlement/days/:businessDate", () => new HttpResponse("404 page not found", { status: 404 })));
    renderWithIntl(<SettlementScreen businessDate="2026-09-21" />);
    expect(await screen.findByRole("heading", { name: "Chưa có dịch vụ đối soát trên hệ thống này" })).toBeInTheDocument();
    expect(screen.getByText(/404 page not found/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Chạy bước/ })).not.toBeInTheDocument();
  });
});
