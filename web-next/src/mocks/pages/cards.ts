import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";
import { MOCK_CARDS, MOCK_LEDGERS } from "../journey-fixtures";

type CardDetail = components["schemas"]["CardDetail"];

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

/** Cards and Accounts (and the POS tiles and Journey balances): the Issuer Admin card API. */
export const cardsHandlers = [
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
