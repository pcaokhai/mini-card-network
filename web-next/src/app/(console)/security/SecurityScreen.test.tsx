import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import en from "../../../../messages/en.json";
import { handlers } from "@/mocks/generated/handlers";
import { SecurityScreen } from "./SecurityScreen";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <SecurityScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("SecurityScreen", () => {
  it("renders the key table, rotation stepper entry point, pin block visualiser, and pci list__MCN_505_AC1_AC2_AC3", () => {
    renderScreen();
    expect(screen.getByRole("heading", { name: /security/i })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /rotate/i }).length).toBeGreaterThan(0);
    expect(screen.getByText(/illustration only/i)).toBeInTheDocument();
    expect(screen.getByRole("list")).toBeInTheDocument();
  });
});
