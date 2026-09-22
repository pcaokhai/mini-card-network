import { act, fireEvent, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { render } from "@testing-library/react";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import en from "../../../../../messages/en.json";
import { handlers } from "@/mocks/generated/handlers";
import type { Journey } from "@/shared/api/journey-client";
import { JourneyScreen } from "./JourneyScreen";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const FIXED_JOURNEY: Journey = {
  transaction: {
    rrn: "some-rrn",
    status: "APPROVED",
    type: "PURCHASE",
    responseCode: "00",
    responseLabel: "Approved",
    amount: { amount: 10000, currency: "704" },
    maskedPan: "970436******4417",
    terminalId: "00000042",
    merchantName: "Ca phe Goc Pho",
    createdAt: new Date().toISOString(),
  },
  steps: [
    { seq: 0, actor: "POS", offsetMs: 0, title: "Purchase requested", easyText: "", technicalText: "", kind: "INFO", message: null },
    { seq: 1, actor: "ISSUER", offsetMs: 50, title: "Approved", easyText: "", technicalText: "", kind: "OK", message: null },
    { seq: 2, actor: "ACQUIRER", offsetMs: 60, title: "Response returned", easyText: "", technicalText: "", kind: "INFO", message: null },
  ] as Journey["steps"],
  money: [{ label: "Purchase", delta: -10000, balanceAfter: null, atStep: 1 }],
};

const REVERSED_JOURNEY: Journey = {
  transaction: { ...FIXED_JOURNEY.transaction, rrn: "reversed-rrn", status: "REVERSED" },
  steps: [
    { seq: 0, actor: "POS", offsetMs: 0, title: "Purchase requested", easyText: "", technicalText: "", kind: "INFO", message: null },
    { seq: 1, actor: "ISSUER", offsetMs: 30000, title: "No response from issuer", easyText: "", technicalText: "", kind: "WARN", message: null },
    { seq: 2, actor: "POS", offsetMs: 30100, title: "Reversal queued", easyText: "", technicalText: "", kind: "WARN", message: null },
    { seq: 3, actor: "SAF", offsetMs: 35000, title: "Money returned", easyText: "", technicalText: "", kind: "REVERSAL", message: null },
  ] as Journey["steps"],
  money: [
    { label: "Purchase", delta: -10000, balanceAfter: null, atStep: 0 },
    { label: "Refund", delta: 10000, balanceAfter: null, atStep: 3 },
  ],
};

function renderScreen(rrn: string, journey: Journey = FIXED_JOURNEY) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  client.setQueryData(["transactions", rrn, "journey"], journey);
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <JourneyScreen rrn={rrn} />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

describe("JourneyScreen", () => {
  it("shows the summary, timeline, and money panel once loaded", async () => {
    renderScreen("some-rrn");
    expect(await screen.findByRole("list", { name: /step timeline/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /money movement/i })).toBeInTheDocument();
  });

  it("autoplay advances the current step on an interval", async () => {
    renderScreen("some-rrn");
    const timeline = await screen.findByRole("list", { name: /step timeline/i });
    vi.useFakeTimers();
    const items = () => within(timeline).getAllByRole("listitem");
    const initialIndex = items().findIndex((el) => el.getAttribute("data-state") === "current");

    fireEvent.click(screen.getByTestId("journey-toggle-play"));
    act(() => {
      vi.advanceTimersByTime(2100);
    });

    const advancedIndex = items().findIndex((el) => el.getAttribute("data-state") === "current");
    expect(advancedIndex).toBeGreaterThan(initialIndex);
    vi.useRealTimers();
  });

  it("shows the countdown ring only for a TIMED_OUT step while autoplaying (MCN-406-AC2)", async () => {
    renderScreen("reversed-rrn", REVERSED_JOURNEY);
    await screen.findByRole("list", { name: /step timeline/i });
    expect(screen.queryByTestId("countdown-ring")).not.toBeInTheDocument();

    vi.useFakeTimers();
    fireEvent.click(screen.getByTestId("journey-toggle-play"));
    act(() => {
      vi.advanceTimersByTime(2100);
    });

    expect(screen.getByTestId("countdown-ring")).toBeInTheDocument();
    vi.useRealTimers();
  });
});
