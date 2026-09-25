import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { labHandlers } from "@/mocks/pages/lab";
import { useDisplayMode } from "@/shared/state/display-mode";
import { renderWithIntl } from "@/test/render";
import { MessageLabScreen } from "./MessageLabScreen";

const server = setupServer(...labHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const detail = () => screen.getByRole("region", { name: "Chi tiết" });
const fieldList = () => screen.getByRole("region", { name: "Danh sách field" });

async function renderLoaded() {
  renderWithIntl(<MessageLabScreen />);
  await screen.findByRole("button", { name: "Bit 11, bật" });
}

describe("MessageLabScreen", () => {
  beforeEach(() => act(() => useDisplayMode.setState({ mode: "easy" })));

  it("opens the 0200 sample on its amount, as the canvas does", async () => {
    await renderLoaded();
    expect(screen.getByRole("button", { name: "0200 Mua hàng" })).toHaveAttribute("aria-pressed", "true");
    expect(within(detail()).getByRole("heading", { name: "Số tiền" })).toBeInTheDocument();
  });

  it("selecting a raw segment highlights the same field in the grid and the list __MCN_104_AC1", async () => {
    await renderLoaded();
    await userEvent.click(screen.getByRole("button", { name: "000123" }));
    expect(within(detail()).getByRole("heading", { name: "Số thứ tự giao dịch" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bit 11, bật" })).toHaveAttribute("aria-pressed", "true");
    expect(within(fieldList()).getByRole("button", { name: /^11\D/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("selecting a bitmap cell or a list row drives the same selection __MCN_104_AC1", async () => {
    await renderLoaded();
    await userEvent.click(within(fieldList()).getByRole("button", { name: /^37\D/ }));
    expect(screen.getByRole("button", { name: "626514000123" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(screen.getByRole("button", { name: "Bit 39, tắt" }));
    expect(within(detail()).getByText("Không có trong message")).toBeInTheDocument();
  });

  it("shows hex and binary per bitmap row and opens the secondary page only when bit 1 is set __MCN_104_AC2", async () => {
    await renderLoaded();
    expect(screen.getAllByRole("button", { name: /^Bit \d+, / })).toHaveLength(64);
    expect(screen.getByText("72")).toBeInTheDocument();
    expect(screen.getByText("01110010")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Phụ 65–128" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "0420 Hủy" }));
    await screen.findByRole("button", { name: "Bit 1, bật" });
    await userEvent.click(screen.getByRole("button", { name: "Phụ 65–128" }));
    expect(screen.getByRole("button", { name: "Bit 90, bật" })).toBeInTheDocument();
    expect(screen.getByText("01000000")).toBeInTheDocument();
  });

  it("breaks down the MTI's four digits __MCN_104_AC3", async () => {
    await renderLoaded();
    await userEvent.click(screen.getByRole("button", { name: "0200" }));
    expect(within(detail()).getByText("Tài chính")).toBeInTheDocument();
    expect(within(detail()).getByText("Do bên thanh toán (acquirer) gửi")).toBeInTheDocument();
  });

  it("names fields in plain words and hides the format column in Easy mode __MCN_104_AC4", async () => {
    await renderLoaded();
    expect(within(fieldList()).getByText("Ý nghĩa")).toBeInTheDocument();
    expect(within(fieldList()).queryByText("Định dạng")).not.toBeInTheDocument();
    expect(within(fieldList()).getByText("Mã tra soát")).toBeInTheDocument();
  });

  it("names fields technically and shows the format column in Expert mode __MCN_104_AC4", async () => {
    act(() => useDisplayMode.setState({ mode: "expert" }));
    await renderLoaded();
    expect(within(fieldList()).getByText("Định dạng")).toBeInTheDocument();
    expect(within(fieldList()).getByText("Retrieval reference number (RRN)")).toBeInTheDocument();
    expect(screen.getByText(/Packager ASCII \(iso87ascii\).*tổng 19 field/)).toBeInTheDocument();
  });

  it("re-flips the bitmap cells along the diagonal when the message changes __MCN_104_AC5", async () => {
    await renderLoaded();
    const before = screen.getByRole("button", { name: "Bit 11, bật" });
    expect(before).toHaveStyle({ animationDelay: `${(1 + 2) * 16}ms` });
    await userEvent.click(screen.getByRole("button", { name: "0210 Trả lời" }));
    await screen.findByRole("button", { name: "Bit 39, bật" });
    expect(screen.getByRole("button", { name: "Bit 11, bật" })).not.toBe(before);
  });
});
