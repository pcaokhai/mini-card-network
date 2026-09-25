import { http, HttpResponse, passthrough } from "msw";
import type { components } from "@/shared/api/generated/schema";
import { APPROVED_JOURNEY, JOURNEYS, MOCK_CARDS, MOCK_LEDGERS } from "./journey-fixtures";

type CardDetail = components["schemas"]["CardDetail"];
type TransactionSummary = components["schemas"]["TransactionSummary"];

const vnd = (amount: number) => ({ amount, currency: "704" });

function cardDetail(cardRef: string): CardDetail | null {
  const card = MOCK_CARDS.find((c) => c.cardRef === cardRef);
  if (!card) return null;
  const { balance, ...summary } = card;
  return {
    ...summary,
    ledgerBalance: vnd(balance),
    availableBalance: vnd(balance),
    holds: [],
    limits: { dailyAmount: vnd(20_000_000), perTransactionAmount: vnd(10_000_000), dailyCount: null },
    usedToday: vnd(0),
  };
}

function summaryOf(rrn: string): TransactionSummary {
  const { transaction } = JOURNEYS[rrn];
  return { ...transaction, stan: rrn.slice(-6), latencyMs: null };
}

/**
 * The Journey screen's data under `pnpm dev:mock`: its journeys, the newest transaction per
 * status, and the Issuer Admin card and ledger calls that give "before" and "after" balances.
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
  http.get("*/v1/cards", () =>
    HttpResponse.json(MOCK_CARDS.map(({ cardRef, maskedPan, holderName, status, expiry }) => ({ cardRef, maskedPan, holderName, status, expiry }))),
  ),
  http.get("*/v1/cards/:cardRef", ({ params }) => {
    const detail = cardDetail(String(params.cardRef));
    return detail ? HttpResponse.json(detail) : new HttpResponse(null, { status: 404 });
  }),
  http.get("*/v1/cards/:cardRef/ledger", ({ params }) =>
    HttpResponse.json({ items: MOCK_LEDGERS[String(params.cardRef)] ?? [], nextCursor: null }),
  ),
];
