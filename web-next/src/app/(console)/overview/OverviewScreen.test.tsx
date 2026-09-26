import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { http, HttpResponse, ws } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import vi from "../../../../messages/vi.json";
import { handlers } from "@/mocks/generated/handlers";
import { scenarioHandlers } from "@/mocks/scenario-handlers";
import { useDisplayMode } from "@/shared/state/display-mode";
import { OverviewScreen } from "./OverviewScreen";

// The live feed opens the gateway stream; accept it silently so strict mode only flags REST gaps.
const stream = ws.link(/\/v1\/stream$/);
const server = setupServer(...handlers, stream.addEventListener("connection", () => {}));
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="vi" messages={vi}>
        <OverviewScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("OverviewScreen", () => {
  beforeEach(() => {
    useDisplayMode.setState({ mode: "easy" });
  });

  it("shows KPIs, throughput, decline reasons, health, and the live feed", async () => {
    renderScreen();
    expect(await screen.findByTestId("kpi-ledger")).toBeInTheDocument();
    expect(screen.getByTestId("live-feed-list")).toBeInTheDocument();
  });

  it("hides Expert-only feed columns in Easy mode", async () => {
    renderScreen();
    await screen.findByTestId("kpi-ledger");
    expect(document.querySelector("[data-expert-only]")).not.toBeInTheDocument();
  });

  it("MCN-306: the dev:mock scenario fills every card the design canvas shows", async () => {
    server.use(...scenarioHandlers);
    renderScreen();

    expect(await screen.findByText("Cà phê Góc Phố")).toBeInTheDocument();
    expect(screen.getAllByTestId("feed-status")).toHaveLength(7);
    expect(screen.getByText("Đã tự hủy")).toBeInTheDocument();
    expect(screen.getByText("Đang chờ")).toBeInTheDocument();
    expect(await screen.findByTestId("health-link")).toBeInTheDocument();
    expect(await screen.findByTestId("health-saf")).toBeInTheDocument();
    expect(await screen.findByTestId("health-stip")).toBeInTheDocument();
    expect(await screen.findByTestId("health-key")).toBeInTheDocument();
    expect(screen.getByText("Tăng 12% so với hôm qua")).toBeInTheDocument();
  });

  it("MCN-306-AC3: Expert mode shows RRN, RC codes and the p50 beside p99", async () => {
    server.use(...scenarioHandlers);
    useDisplayMode.setState({ mode: "expert" });
    renderScreen();

    expect(await screen.findByText("626514000123")).toBeInTheDocument();
    expect(screen.getByText("RRN (DE 37)")).toBeInTheDocument();
    expect(screen.getByText("Không đủ tiền · RC 51", { selector: "[data-testid=feed-status]" })).toBeInTheDocument();
    expect(screen.getByText("p99 end-to-end · p50 96 ms")).toBeInTheDocument();
  });

  it("shows a loading state, then the business date as today __OVW_G8_ADR_007", async () => {
    server.use(...scenarioHandlers);
    renderScreen();
    expect(screen.getByRole("status", { name: "Đang tải tổng quan…" })).toBeInTheDocument();

    expect(await screen.findByText(/Thứ Sáu, 25\/09\/2026 · ngày giao dịch/)).toBeInTheDocument();
  });

  it("shows a problem banner when the overview can't be loaded __OVW_G8", async () => {
    server.use(http.get("*/v1/metrics/overview", () => HttpResponse.json({ type: "internal", status: 500 }, { status: 500 })));
    renderScreen();

    expect(await screen.findByRole("alert")).toHaveTextContent("Không tải được số liệu tổng quan. Thử tải lại trang.");
    expect(screen.queryByTestId("kpi-ledger")).not.toBeInTheDocument();
  });
});
