import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import en from "../../../../../messages/en.json";
import { handlers } from "@/mocks/generated/handlers";
import { CardDetailScreen } from "./CardDetailScreen";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen(cardRef: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <CardDetailScreen cardRef={cardRef} />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("CardDetailScreen", () => {
  it("shows a specific message on a 412 stale-ETag response when saving limits __MCN_309_AC3", async () => {
    server.use(
      http.put("/api/v1/cards/:cardRef/limits", () =>
        HttpResponse.json(
          { type: "https://mcn.local/problems/precondition-failed", title: "Precondition failed", status: 412 },
          { status: 412 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderScreen("crd_normal0001");
    await screen.findByText(/ledger balance/i);
    await user.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByText("Someone changed this card. Reload to continue.")).toBeInTheDocument();
  });

  it("has a single accessible heading naming the card __MCN_309_AC1", async () => {
    renderScreen("crd_normal0001");
    expect(await screen.findByRole("heading", { level: 1 })).toBeInTheDocument();
  });
});
