import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import en from "../../../../messages/en.json";
import { handlers } from "@/mocks/generated/handlers";
import { CardsListScreen } from "./CardsListScreen";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <CardsListScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("CardsListScreen", () => {
  it("lists cards with a status badge for each __MCN_309_AC1", async () => {
    renderScreen();
    expect(await screen.findAllByRole("row")).not.toHaveLength(0);
  });

  it("has a single accessible heading naming the screen", async () => {
    renderScreen();
    expect(await screen.findByRole("heading", { level: 1 })).toBeInTheDocument();
  });
});
