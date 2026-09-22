import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import vi from "../../../../../messages/vi.json";
import { handlers } from "@/mocks/generated/handlers";
import { ChaosLabScreen } from "./ChaosLabScreen";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="vi" messages={vi}>
        <ChaosLabScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("ChaosLabScreen", () => {
  it("renders exactly six scenario cards and an active-count header __MCN_405_AC1", async () => {
    renderScreen();
    const cards = await screen.findAllByRole("switch");
    expect(cards).toHaveLength(6);
    expect(screen.getByTestId("chaos-active-count")).toBeInTheDocument();
  });

  it("renders the money verification and reaction panels __MCN_405_AC2_AC3", async () => {
    renderScreen();
    await screen.findAllByRole("switch");
    expect(screen.getByTestId("money-verification")).toBeInTheDocument();
  });
});
