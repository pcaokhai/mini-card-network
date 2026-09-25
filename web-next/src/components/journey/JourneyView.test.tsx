import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import en from "../../../messages/en.json";
import { journeyHandlers } from "@/mocks/journey-handlers";
import { cardsHandlers } from "@/mocks/pages/cards";
import { APPROVED_JOURNEY, AUTO_REVERSED_JOURNEY, DECLINED_JOURNEY } from "@/mocks/journey-fixtures";
import { useDisplayMode } from "@/shared/state/display-mode";
import { AUTOPLAY_INTERVAL_MS, JourneyView } from "./JourneyView";

const server = setupServer(...journeyHandlers, ...cardsHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  vi.useRealTimers();
});
afterAll(() => server.close());
beforeEach(() => useDisplayMode.setState({ mode: "easy" }));

function renderJourney(rrn: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <NextIntlClientProvider locale="en" messages={en}>
        <JourneyView rrn={rrn} />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

const steps = () => within(screen.getByRole("region", { name: "Steps" })).getAllByRole("button", { name: /./ }).filter((b) => b.classList.contains("journey-step"));
const currentIndex = () => steps().findIndex((b) => b.getAttribute("aria-current") === "step");

describe("JourneyView (MCN-307: summary, timeline, detail, money as in the design)", () => {
  it("summarises the outcome, total time, message count and the customer's money __MCN_307_AC1", async () => {
    renderJourney(APPROVED_JOURNEY.transaction.rrn);

    const summary = await screen.findByRole("region", { name: "Transaction summary" });
    expect(within(summary).getByText("Purchase of 250.000 ₫ approved")).toBeInTheDocument();
    expect(within(summary).getByText("Cà phê Góc Phố · Card •••• 4417 · Reference 626514000123")).toBeInTheDocument();
    expect(within(summary).getByText("182 ms")).toBeInTheDocument();
    expect(within(summary).getByText("2")).toBeInTheDocument(); // 0200 and 0210
    expect(within(summary).getByText("250.000 ₫ debited")).toBeInTheDocument();
  });

  it("opens on the final step, renders each step's copy from its code, and shows its ISO message __MCN_307_AC1", async () => {
    renderJourney(APPROVED_JOURNEY.transaction.rrn);
    await screen.findByText("The POS prints the receipt", { selector: "h2" });

    expect(currentIndex()).toBe(3);
    fireEvent.click(steps()[2]);
    const detail = screen.getByRole("region", { name: "Step detail" });
    expect(within(detail).getByRole("heading", { name: "The issuer approves" })).toBeInTheDocument();
    const fields = within(detail).getByRole("table", { name: "Message 0210 content" });
    expect(within(fields).getByText("Approval code")).toBeInTheDocument();
    expect(within(fields).getByText("A00123")).toBeInTheDocument();
  });

  it("times the receipt at its own step, not at the end of a later cancellation", async () => {
    server.use(
      http.get("*/v1/transactions/:rrn/journey", () =>
        HttpResponse.json({ ...APPROVED_JOURNEY, steps: [...APPROVED_JOURNEY.steps, { ...AUTO_REVERSED_JOURNEY.steps[6], seq: 5, offsetMs: 636 }] }),
      ),
    );
    renderJourney(APPROVED_JOURNEY.transaction.rrn);

    expect(await screen.findByText("The customer gets the goods. The whole journey took 182 ms.")).toBeInTheDocument();
  });

  it("switches the timeline and field names to technical copy in Expert mode", async () => {
    useDisplayMode.setState({ mode: "expert" });
    renderJourney(APPROVED_JOURNEY.transaction.rrn);
    fireEvent.click((await screen.findAllByText("Issuer host (jPOS)"))[0]);

    expect(screen.getByText("RRN 626514000123", { exact: false })).toBeInTheDocument();
    expect(screen.getAllByText("0210 · RC 00 · field 38 = A00123").length).toBeGreaterThan(0);
    expect(screen.getByText("Message 0210 · 11 elements")).toBeInTheDocument();
    expect(screen.getByText("F38")).toBeInTheDocument();
    expect(screen.getByText("Authorization ID")).toBeInTheDocument();
  });

  it("dims steps after the current one and steps back and forward __MCN_307_AC2", async () => {
    renderJourney(APPROVED_JOURNEY.transaction.rrn);
    await screen.findByText("The POS prints the receipt", { selector: "h2" });

    fireEvent.click(screen.getByTestId("journey-restart"));
    expect(currentIndex()).toBe(0);
    expect(steps()[1]).toHaveAttribute("data-future", "true");
    expect(screen.getByTestId("journey-back")).toBeDisabled();
    fireEvent.click(screen.getByTestId("journey-next"));
    expect(currentIndex()).toBe(1);
  });

  it("autoplays from the first step at the canvas's pace __MCN_307_AC2", async () => {
    renderJourney(APPROVED_JOURNEY.transaction.rrn);
    await screen.findByText("The POS prints the receipt", { selector: "h2" });
    vi.useFakeTimers();

    fireEvent.click(screen.getByTestId("journey-toggle-play"));
    expect(currentIndex()).toBe(0);
    act(() => vi.advanceTimersByTime(AUTOPLAY_INTERVAL_MS));
    expect(currentIndex()).toBe(1);
  });

  it("plays the 30 s timeout as an accelerated countdown __MCN_406_AC2", async () => {
    renderJourney(AUTO_REVERSED_JOURNEY.transaction.rrn);
    await screen.findByText("The issuer returns the money", { selector: "h2" });
    expect(screen.queryByTestId("countdown-ring")).not.toBeInTheDocument();
    vi.useFakeTimers();

    fireEvent.click(screen.getByTestId("journey-toggle-play"));
    // Each tick schedules the next one after React re-renders, so advance one step per act.
    act(() => vi.advanceTimersByTime(AUTOPLAY_INTERVAL_MS));
    act(() => vi.advanceTimersByTime(AUTOPLAY_INTERVAL_MS));

    expect(screen.getByRole("heading", { name: "No answer in time" })).toBeInTheDocument();
    expect(screen.getByTestId("countdown-ring")).toHaveAttribute("data-playing", "true");
  });

  it("shows the issuer's debit then refund between the balance before and after __MCN_406_AC1", async () => {
    renderJourney(AUTO_REVERSED_JOURNEY.transaction.rrn);
    const money = await screen.findByRole("region", { name: "Money in the customer's account" });

    expect(await within(money).findAllByText("200.000.000 ₫")).toHaveLength(2); // before and final
    expect(within(money).getByText("−600.000 ₫").closest("[data-kind]")).toHaveAttribute("data-kind", "debit");
    expect(within(money).getByText("+600.000 ₫").closest("[data-kind]")).toHaveAttribute("data-kind", "credit");
    expect(screen.getByText("Nothing lost")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("journey-restart"));
    expect(within(money).getByText("+600.000 ₫").closest("[data-kind]")).toHaveAttribute("data-reached", "false");
  });

  it("tells a decline apart: the issuer's reason, and no money moved", async () => {
    renderJourney(DECLINED_JOURNEY.transaction.rrn);
    fireEvent.click((await screen.findAllByText("The issuer declines"))[0]);

    expect(screen.getAllByText("Reason: Insufficient funds. Nothing was debited.").length).toBeGreaterThan(0);
    expect(screen.getByText("Nothing debited")).toBeInTheDocument();
    expect(screen.queryByText("Before the transaction")).not.toBeInTheDocument();
  });
});
