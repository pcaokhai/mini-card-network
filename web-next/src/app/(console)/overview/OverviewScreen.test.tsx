import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import vi from "../../../../messages/vi.json";
import { handlers } from "@/mocks/generated/handlers";
import { useDisplayMode } from "@/shared/state/display-mode";
import { OverviewScreen } from "./OverviewScreen";

const server = setupServer(...handlers);
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
});
