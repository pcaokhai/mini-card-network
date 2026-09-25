import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { MOCK_ROTATION_STEP_MS, resetSecurityMock, securityHandlers } from "@/mocks/pages/security";
import { useDisplayMode } from "@/shared/state/display-mode";
import { renderWithIntl } from "@/test/render";
import { SecurityScreen } from "./SecurityScreen";

const server = setupServer(...securityHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => {
  resetSecurityMock();
  useDisplayMode.setState({ mode: "easy" });
});
afterEach(() => {
  server.resetHandlers();
  vi.useRealTimers();
});
afterAll(() => server.close());

const region = (name: string) => screen.getByRole("region", { name });

async function keyRowsText() {
  const list = region("Danh sách khóa");
  await within(list).findByText("8C21D4");
  const rows = within(list).getAllByRole("row");
  return rows.map((r) => r.textContent);
}

describe("SecurityScreen (MCN-505, as in Security.dc.html)", () => {
  it("lists every key with its KCV, days left and status in Easy wording MCN-505-AC1", async () => {
    renderWithIntl(<SecurityScreen />);
    expect(screen.getByRole("heading", { level: 1, name: "Bảo mật và khóa" })).toBeInTheDocument();
    expect(screen.getByText("Mã kiểm tra giúp hai bên so khóa mà không lộ khóa")).toBeInTheDocument();

    const rows = await keyRowsText();
    expect(rows).toHaveLength(7); // header + 6 keys
    expect(rows[0]).toBe("KhóaMã kiểm traThời hạn còn lạiTrạng tháiThao tác");
    expect(rows[1]).toBe("Khóa chủ giữa hai ngân hàngBọc các khóa khác khi trao đổi8C21D4Còn 312 ngàyĐang dùng");
    expect(rows[2]).toBe("Khóa mã hóa PIN giữa hai ngân hàngBảo vệ PIN trên đường truyền liên ngân hàng3F9A21Còn 26 ngàyĐang dùngXoay khóa ngay");
    expect(rows[3]).toBe("Khóa ký chống giả mạoTính chữ ký MAC cho field 6451E0A7Còn 5 ngàySắp đến hạn");
    expect(rows[6]).toBe("Khóa xác thực PINTạo và kiểm tra giá trị PVVD19E40Còn 200 ngàyĐang dùng");
  });

  it("names keys by their technical type and lifecycle in Expert mode MCN-505-AC1", async () => {
    useDisplayMode.setState({ mode: "expert" });
    renderWithIntl(<SecurityScreen />);
    expect(screen.getByText("Hiển thị KCV · key material không rời HSM")).toBeInTheDocument();

    const rows = await keyRowsText();
    expect(rows[2]).toBe("ZPK · Khóa mã hóa PIN giữa hai ngân hàngunder LMK · 30 ngày/chu kỳ3F9A21Còn 26 ngàyACTIVEXoay khóa ngay");
    expect(rows[3]).toContain("ROTATE SOON");
  });

  it("walks the rotation through its four steps and flips the ZPK check value MCN-505-AC1", async () => {
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    renderWithIntl(<SecurityScreen />);
    const rotation = region("Xoay khóa");
    const steps = () => within(rotation).getAllByRole("listitem").map((li) => li.getAttribute("data-status"));
    expect(steps()).toEqual(["PENDING", "PENDING", "PENDING", "PENDING"]);

    await userEvent.click(await screen.findByRole("button", { name: "Xoay khóa ngay" }));
    expect(await within(rotation).findByRole("button", { name: "Đang xoay…" })).toBeDisabled();
    expect(steps()).toEqual(["DONE", "PENDING", "PENDING", "PENDING"]);
    expect(screen.queryByRole("button", { name: "Xoay khóa ngay" })).toBeNull();

    act(() => vi.setSystemTime(Date.now() + 4 * MOCK_ROTATION_STEP_MS));
    expect(await within(rotation).findByRole("button", { name: "Hoàn tất · làm lại" }, { timeout: 3000 })).toBeEnabled();
    expect(steps()).toEqual(["DONE", "DONE", "DONE", "DONE"]);
    const kcv = await screen.findByText("7D02B1", { selector: "[data-flip]" });
    expect(kcv).toHaveAttribute("data-flip", "true");
    // The key list now carries the new check value in place of the old one.
    expect(screen.queryByText("3F9A21")).toBeNull();

    await userEvent.click(within(rotation).getByRole("button", { name: "Hoàn tất · làm lại" }));
    expect(steps()).toEqual(["PENDING", "PENDING", "PENDING", "PENDING"]);
    expect(within(rotation).getByRole("button", { name: "Bắt đầu xoay khóa ZPK" })).toBeEnabled();
  });

  it("shows each rotation step's detail in Easy and Expert wording MCN-505-AC1", async () => {
    renderWithIntl(<SecurityScreen />);
    expect(within(region("Xoay khóa")).getByText("Khóa mới được bọc bằng khóa chủ trước khi gửi qua mạng.")).toBeInTheDocument();
    act(() => useDisplayMode.setState({ mode: "expert" }));
    expect(within(region("Xoay khóa")).getByText("0800 · field 70 = 161 · ZPK under ZMK")).toBeInTheDocument();
  });

  it("reports a rotation the gateway refuses MCN-505-AC1", async () => {
    server.use(
      http.post("*/v1/keys/acquirer/rotations", () =>
        HttpResponse.json({ type: "about:blank", title: "Conflict", status: 409 }, { status: 409 }),
      ),
    );
    renderWithIntl(<SecurityScreen />);
    await userEvent.click(within(region("Xoay khóa")).getByRole("button", { name: "Bắt đầu xoay khóa ZPK" }));
    expect(await within(region("Xoay khóa")).findByRole("alert")).toHaveTextContent("Không bắt đầu xoay khóa được");
  });

  it("stops waiting when the rotation can no longer be read MCN-505-AC1", async () => {
    server.use(http.get("*/v1/keys/acquirer/rotations/:rotationId", () => new HttpResponse(null, { status: 404 })));
    renderWithIntl(<SecurityScreen />);
    const rotation = region("Xoay khóa");
    await userEvent.click(within(rotation).getByRole("button", { name: "Bắt đầu xoay khóa ZPK" }));
    expect(await within(rotation).findByRole("alert")).toHaveTextContent("Không theo dõi được lần xoay khóa này");
    expect(within(rotation).getByRole("button", { name: "Bắt đầu xoay khóa ZPK" })).toBeEnabled();
  });

  it("builds the PIN block from a made-up card, labelled as an illustration MCN-505-AC2", async () => {
    renderWithIntl(<SecurityScreen />);
    const pin = region("Mô phỏng PIN block");
    expect(within(pin).getByText("Số thẻ minh họa dùng để tính: 9704 36•• •••• 7890")).toBeInTheDocument();
    expect(within(pin).getByText(/Hai dòng cuối là giá trị minh họa/)).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/4417/);

    // The canvas opens with PIN 1234 already worked through.
    expect(within(pin).getByText("04•• ••FF FFFF FFFF")).toBeInTheDocument();
    expect(within(pin).getByText("0412 779E DCBA 9876")).toBeInTheDocument();

    const input = within(pin).getByLabelText("Nhập thử một mã PIN");
    expect(input).toHaveAttribute("type", "password");
    await userEvent.clear(input);
    await userEvent.type(input, "123456");
    await userEvent.click(within(pin).getByRole("button", { name: "Tạo PIN block" }));
    expect(within(pin).getByText("06•• •••• FFFF FFFF")).toBeInTheDocument();
  });

  it("rejects a PIN that is not 4 to 12 digits with an inline error MCN-505-AC2", async () => {
    renderWithIntl(<SecurityScreen />);
    const pin = region("Mô phỏng PIN block");
    const input = within(pin).getByLabelText("Nhập thử một mã PIN");
    await userEvent.clear(input);
    await userEvent.type(input, "12a");
    await userEvent.click(within(pin).getByRole("button", { name: "Tạo PIN block" }));
    expect(within(pin).getByRole("alert")).toHaveTextContent("Mã PIN gồm 4 đến 12 chữ số, không có chữ cái.");
    expect(within(pin).getByText("04•• ••FF FFFF FFFF")).toBeInTheDocument(); // the last good block stays

    await userEvent.type(input, "4");
    expect(within(pin).queryByRole("alert")).toBeNull();
  });

  it("labels the PIN block rows technically in Expert mode MCN-505-AC2", () => {
    useDisplayMode.setState({ mode: "expert" });
    renderWithIntl(<SecurityScreen />);
    const pin = region("Mô phỏng PIN block");
    expect(within(pin).getByText("Clear PIN block = PIN ⊕ PAN")).toBeInTheDocument();
    expect(within(pin).getByText("Translated TPK → ZPK")).toBeInTheDocument();
  });

  it("lists what the system never does, in Easy and Expert wording MCN-505-AC3", () => {
    renderWithIntl(<SecurityScreen />);
    const pci = region("Tuân thủ PCI DSS");
    expect(within(pci).getByRole("heading", { name: "Những điều hệ thống không bao giờ làm" })).toBeInTheDocument();
    expect(within(pci).getAllByRole("listitem")).toHaveLength(6);
    expect(within(pci).getByText("Không bao giờ lưu mã CVV in sau thẻ.")).toBeInTheDocument();

    act(() => useDisplayMode.setState({ mode: "expert" }));
    expect(within(pci).getByText("Log masker cho field 2, 35, 45, 52, 55")).toBeInTheDocument();
  });
});
