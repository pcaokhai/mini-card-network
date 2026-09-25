import { http, HttpResponse, passthrough } from "msw";
import type { components } from "@/shared/api/generated/schema";
import { APPROVED_JOURNEY, JOURNEYS } from "./journey-fixtures";

type TransactionSummary = components["schemas"]["TransactionSummary"];


function summaryOf(rrn: string): TransactionSummary {
  const { transaction } = JOURNEYS[rrn];
  return { ...transaction, stan: rrn.slice(-6), latencyMs: null };
}

/**
 * The Journey screen's data under `pnpm dev:mock`: its journeys and the newest transaction per
 * status. The card and ledger calls behind its balances are the Cards page's (pages/cards.ts).
 * Registered ahead of the scenario handlers; an unfiltered transaction list falls through to them.
 */
export const journeyHandlers = [
  http.get("*/v1/transactions", ({ request }) => {
    const status = new URL(request.url).searchParams.get("status");
    if (!status) return passthrough();
    const items = Object.keys(JOURNEYS)
      .filter((rrn) => JOURNEYS[rrn].transaction.status === status)
      .map(summaryOf);
    return HttpResponse.json({ items, nextCursor: null });
  }),
  http.get("*/v1/transactions/:rrn/journey", ({ params }) => {
    const rrn = String(params.rrn);
    // Any other RRN (the overview feed, a POS result) replays the approved story under its own number.
    const journey = JOURNEYS[rrn] ?? { ...APPROVED_JOURNEY, transaction: { ...APPROVED_JOURNEY.transaction, rrn } };
    return HttpResponse.json(journey);
  }),
];
