import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import en from "../../../../messages/en.json";
import { journeyHandlers } from "@/mocks/journey-handlers";
import { JourneyIndexScreen } from "./JourneyIndexScreen";

const replace = vi.fn();
let search = new URLSearchParams();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
  useSearchParams: () => search,
}));

const server = setupServer(...journeyHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
beforeEach(() => {
  replace.mockReset();
  search = new URLSearchParams();
});

function renderIndex() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <JourneyIndexScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("/transactions (was a 'coming in R3' placeholder)", () => {
  it("opens on the newest approved transaction's journey", async () => {
    renderIndex();

    expect(await screen.findByText("Purchase of 250.000 ₫ approved")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approved" })).toHaveAttribute("aria-pressed", "true");
  });

  it("follows the tab in the URL, so a reversal can be shared", async () => {
    search = new URLSearchParams("view=reversed");
    renderIndex();

    expect(await screen.findByText("Transaction of 600.000 ₫ reversed automatically")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Declined" }));
    expect(replace).toHaveBeenCalledWith("/transactions?view=declined");
  });

  it("points to the POS when there is no transaction of that kind yet", async () => {
    server.use(http.get("*/v1/transactions", () => HttpResponse.json({ items: [], nextCursor: null })));
    renderIndex();

    expect(await screen.findByText("No transaction of this kind yet")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the POS" })).toHaveAttribute("href", "/pos");
  });
});
