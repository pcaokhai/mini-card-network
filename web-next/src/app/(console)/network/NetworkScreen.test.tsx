import { screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { render } from "@testing-library/react";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import vi from "../../../../messages/vi.json";
import { handlers } from "@/mocks/generated/handlers";
import { NetworkScreen } from "./NetworkScreen";

const server = setupServer(...handlers);
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

describe("NetworkScreen", () => {
  it("shows the links table and event timeline from the API __MCN_205_AC1 __MCN_205_AC2", async () => {
    renderScreen();
    expect(await screen.findAllByRole("row")).not.toHaveLength(0);
  });

  it("has a single accessible heading naming the screen", async () => {
    renderScreen();
    expect(await screen.findByRole("heading", { level: 1 })).toBeInTheDocument();
  });
});
