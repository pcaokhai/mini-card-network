import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { http, HttpResponse, ws } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import vi from "../../../../messages/vi.json";
import { handlers } from "@/mocks/generated/handlers";
import { networkHandlers, resetNetworkMock } from "@/mocks/pages/network";
import { useDisplayMode } from "@/shared/state/display-mode";
import { NetworkScreen } from "./NetworkScreen";

const stream = ws.link(/\/v1\/stream$/);
const server = setupServer(...networkHandlers, ...handlers, stream.addEventListener("connection", () => {}));
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="vi" messages={vi}>
        <NetworkScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

const section = (name: string) => screen.getByRole("region", { name });

describe("NetworkScreen", () => {
  beforeEach(() => {
    resetNetworkMock();
    useDisplayMode.setState({ mode: "easy" });
  });

  it("MCN-205-AC1: the topology shows POS, acquirer, switch and issuer with flowing links", async () => {
    renderScreen();
    const map = section("Sơ đồ mạng");
    expect(await within(map).findByText("42 máy đã đăng ký")).toBeInTheDocument();
    expect(await within(map).findByText("2 máy chủ gateway")).toBeInTheDocument();
    expect(await within(map).findByText("Định tuyến theo đầu số thẻ")).toBeInTheDocument();
    expect(within(map).getByText("2 máy chủ đang chạy")).toBeInTheDocument();
    expect(map.querySelectorAll('[data-state="up"]')).toHaveLength(3);
  });

  it("MCN-205-AC2: each link row shows status, latency and last echo, and Check now echoes it", async () => {
    renderScreen();
    const table = await screen.findByRole("table");
    const row = await within(table).findByRole("row", { name: /Gateway A → Bộ chuyển mạch/ });
    expect(within(row).getByText("Đã đăng nhập")).toBeInTheDocument();
    expect(within(row).getByText("4 ms")).toBeInTheDocument();
    expect(within(row).getByText(/giây trước/)).toBeInTheDocument();
    expect(within(table).getAllByRole("row")).toHaveLength(5);

    await userEvent.click(within(row).getByRole("button", { name: "Kiểm tra ngay" }));
    expect(await within(row).findByText("Vừa xong")).toBeInTheDocument();
  });

  it("MCN-205-AC3: the event log lists today's events newest first", async () => {
    renderScreen();
    const log = section("Nhật ký sự kiện");
    const items = await within(log).findAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent("14:30:12");
    expect(items[0]).toHaveTextContent("Xoay khóa mã hóa PIN thành công");
  });

  it("MCN-205-AC3: a network.event pushed over WS refetches the log without waiting for the poll", async () => {
    const pushed = { id: "ws-1", occurredAt: new Date().toISOString(), severity: "WARN", easyText: "Đường tới issuer chậm", technicalText: "echo slow" };
    let calls = 0;
    server.use(
      http.get("*/v1/network/events", () => {
        calls += 1;
        return HttpResponse.json({ items: calls === 1 ? [] : [pushed], nextCursor: null });
      }),
      stream.addEventListener("connection", ({ client }) => {
        setTimeout(() => client.send(JSON.stringify({ id: "1", type: "network.event", occurredAt: pushed.occurredAt, data: pushed })), 50);
      }),
    );
    renderScreen();
    expect(await screen.findByText("Hôm nay chưa có sự kiện mạng nào.")).toBeInTheDocument();
    expect(await screen.findByText("Đường tới issuer chậm")).toBeInTheDocument();
  });

  it("MCN-205-AC3: gateway event texts show in Vietnamese; unknown text and Expert keep the raw gateway text", async () => {
    const at = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    server.use(
      http.get("*/v1/network/events", () =>
        HttpResponse.json({
          items: [
            { id: "2", occurredAt: at(1), severity: "INFO", easyText: "Link to issuer is up", technicalText: "signed on" },
            { id: "1", occurredAt: at(2), severity: "WARN", easyText: "Brand new gateway text", technicalText: "new" },
          ],
          nextCursor: null,
        }),
      ),
    );
    renderScreen();
    expect(await screen.findByText("Đường kết nối tới ngân hàng phát hành đã hoạt động")).toBeInTheDocument();
    expect(screen.getByText("Brand new gateway text")).toBeInTheDocument();
    useDisplayMode.setState({ mode: "expert" });
    expect(await screen.findByText("signed on")).toBeInTheDocument();
  });

  it("MCN-205-AC3: the log shows the newest five rows and Xem thêm reveals the rest", async () => {
    const items = Array.from({ length: 8 }, (_, i) => ({
      id: String(i),
      occurredAt: new Date(Date.now() - i * 60_000).toISOString(),
      severity: "INFO",
      easyText: `Sự kiện ${i}`,
      technicalText: `event ${i}`,
    }));
    server.use(http.get("*/v1/network/events", () => HttpResponse.json({ items, nextCursor: null })));
    renderScreen();
    const log = section("Nhật ký sự kiện");
    expect(await within(log).findAllByRole("listitem")).toHaveLength(5);
    expect(within(log).getByText("Sự kiện 0")).toBeInTheDocument();
    await userEvent.click(within(log).getByRole("button", { name: "Xem thêm 3 sự kiện" }));
    expect(within(log).getAllByRole("listitem")).toHaveLength(8);
    expect(within(log).queryByRole("button", { name: /Xem thêm/ })).not.toBeInTheDocument();
  });

  it("MCN-804-AC1: breaker pills, STIP tiles and an empty SAF queue in Easy copy", async () => {
    renderScreen();
    const breaker = section("Ngắt mạch và duyệt thay");
    expect(await within(breaker).findByText("500.000 ₫")).toBeInTheDocument();
    expect(within(breaker).getByText("Bình thường")).toHaveAttribute("data-current", "true");
    expect(within(breaker).getByText("Đã ngắt")).toHaveAttribute("data-current", "false");
    const saf = section("Hàng đợi gửi lại");
    expect(await within(saf).findByText("Không có lệnh nào đang chờ. Mọi thông báo đã tới nơi.")).toBeInTheDocument();
    expect(within(saf).getByText("Trống")).toBeInTheDocument();
  });

  it("MCN-804-AC1: Expert mode shows the technical copy from the canvas", async () => {
    useDisplayMode.setState({ mode: "expert" });
    renderScreen();
    expect(await screen.findByText("gateway-a, gateway-b (Go)")).toBeInTheDocument();
    expect(await screen.findByRole("row", { name: /gateway-a → switch/ })).toBeInTheDocument();
    expect(screen.getAllByText("SIGNED_ON")).toHaveLength(4);
    expect(screen.getByRole("columnheader", { name: "p99" })).toBeInTheDocument();
    expect(screen.getByText("Circuit breaker và STIP")).toBeInTheDocument();
    expect(await screen.findByText("depth 0 · dead 0 · oldest —")).toBeInTheDocument();
    expect(await screen.findByText("0800/0810 70=161 · ZPK KCV 3F9A21 ACTIVE")).toBeInTheDocument();
  });

  it("MCN-804-AC2: simulating an issuer outage opens the circuit, fills SAF and breaks the last link", async () => {
    renderScreen();
    await userEvent.click(await screen.findByRole("button", { name: "Mô phỏng ngân hàng phát hành sập" }));

    expect(await screen.findByRole("button", { name: "Khôi phục ngân hàng phát hành" })).toBeInTheDocument();
    const map = section("Sơ đồ mạng");
    await waitFor(() => expect(map.querySelectorAll('[data-state="down"]')).toHaveLength(1));
    expect(within(map).getByText("Đang duyệt thay")).toBeInTheDocument();
    expect(await screen.findAllByText("Mất kết nối")).toHaveLength(2);
    const breaker = section("Ngắt mạch và duyệt thay");
    expect(within(breaker).getByText("Đã ngắt")).toHaveAttribute("data-current", "true");
    expect(within(breaker).getByText("37")).toBeInTheDocument();
    const saf = section("Hàng đợi gửi lại");
    expect(within(saf).getByText("37 lệnh")).toBeInTheDocument();
    expect(within(saf).getByText("Thông báo duyệt thay · 320.000 ₫")).toBeInTheDocument();
    expect(within(saf).getByText("Lệnh hủy · 450.000 ₫")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Khôi phục ngân hàng phát hành" }));
    expect(await screen.findByText(/Ngân hàng phát hành hoạt động trở lại/)).toBeInTheDocument();
    await waitFor(() => expect(map.querySelectorAll('[data-state="up"]')).toHaveLength(3));
  });

  it("shows designed unavailable states where the real gateway has no switch or terminal API", async () => {
    server.use(
      http.get("*/v1/network/switch", () => new HttpResponse("404 page not found", { status: 404 })),
      http.get("*/v1/terminals", () => new HttpResponse("404 page not found", { status: 404 })),
      http.get("*/v1/network/links", () =>
        HttpResponse.json([
          { linkId: "issuer", from: "gateway", to: "issuer", status: "SIGNED_ON", lastEchoAt: null, lastEchoOk: true, p99LatencyMs: null, inFlight: 0 },
        ]),
      ),
    );
    renderScreen();
    const map = section("Sơ đồ mạng");
    expect(await within(map).findByText("Chưa có danh sách máy")).toBeInTheDocument();
    expect(await within(map).findByText("Chưa có dữ liệu")).toBeInTheDocument();
    expect(await screen.findByText(/Bộ chuyển mạch chưa báo trạng thái/)).toBeInTheDocument();
    expect(await screen.findByRole("row", { name: /Gateway → Issuer/ })).toBeInTheDocument();
    expect(within(section("Ngắt mạch và duyệt thay")).getAllByText("—")).toHaveLength(2);
  });

  it("has a single accessible heading naming the screen", async () => {
    renderScreen();
    expect(await screen.findByRole("heading", { level: 1, name: "Vận hành mạng" })).toBeInTheDocument();
  });
});
