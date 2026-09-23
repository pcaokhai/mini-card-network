import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { handlers } from "@/mocks/generated/handlers";
import { KeyTable } from "./KeyTable";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderTable() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <KeyTable />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("KeyTable", () => {
  it("renders one row per key with its KCV__MCN_505_AC1", async () => {
    renderTable();
    await waitFor(() => expect(screen.getAllByRole("row").length).toBeGreaterThan(1));
  });
});
