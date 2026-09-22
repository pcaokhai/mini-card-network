import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { HttpResponse, delay, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import en from "../../../../messages/en.json";
import { handlers } from "@/mocks/generated/handlers";
import { PosScreen } from "./PosScreen";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <PosScreen />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

async function enterPin(user: ReturnType<typeof userEvent.setup>) {
  for (const digit of ["1", "2", "3", "4"]) {
    await user.click(screen.getByRole("button", { name: digit }));
  }
  await user.click(screen.getByRole("button", { name: /confirm/i }));
}

describe("PosScreen", () => {
  it("locks the pay button while processing and shows a result after", async () => {
    const user = userEvent.setup();
    renderScreen();

    server.use(
      http.post("/api/v1/transactions/purchases", async () => {
        await delay(50);
        return HttpResponse.json(
          {
            rrn: "x",
            type: "PURCHASE",
            status: "APPROVED",
            responseCode: "00",
            responseLabel: "Approved",
            amount: { amount: 1000, currency: "704" },
            maskedPan: "970436******4417",
            terminalId: "00000042",
            merchantName: "Ca phe Goc Pho",
            createdAt: new Date().toISOString(),
          },
          { status: 201 },
        );
      }),
    );

    await user.click(screen.getAllByRole("radio")[0]);
    await user.type(screen.getByLabelText(/amount/i), "1000");
    await enterPin(user);

    const payButton = screen.getByRole("button", { name: /^pay$/i });
    expect(payButton).not.toBeDisabled();
    await user.click(payButton);

    expect(payButton).toBeDisabled();
    await waitFor(() => expect(payButton).not.toBeDisabled());
    expect(document.querySelector(".result-panel")).toBeInTheDocument();
  });

  it("has a single accessible heading naming the screen", () => {
    renderScreen();
    expect(screen.getByRole("heading", { level: 1 })).toBeInTheDocument();
  });
});
