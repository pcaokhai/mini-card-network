import { useQuery } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type Journey = components["schemas"]["Journey"];

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// See network-client.ts: pass a thunk so requests use the current global fetch (MSW patches it in tests).
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

export function useJourney(rrn: string) {
  return useQuery({
    queryKey: ["transactions", rrn, "journey"],
    queryFn: async (): Promise<Journey> => {
      const { data, error } = await client.GET("/v1/transactions/{rrn}/journey", {
        params: { path: { rrn } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}

export type TransactionSummary = components["schemas"]["TransactionSummary"];
type TransactionStatus = components["schemas"]["TransactionStatus"];
type JournalEntry = components["schemas"]["JournalEntry"];

/** The newest transaction with this status: the Journey index opens on it. Null when there is none yet. */
export function useLatestTransaction(status: TransactionStatus) {
  return useQuery({
    queryKey: ["transactions", "latest", status],
    queryFn: async (): Promise<TransactionSummary | null> => {
      const { data, error } = await client.GET("/v1/transactions", {
        params: { query: { status, limit: 1 } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data.items[0] ?? null;
    },
  });
}

const LEDGER_PAGE_SIZE = 200;
// ponytail: a card with more than 2 000 journals after this transaction shows no balances; page
// further (or add an rrn filter to the ledger API) if the lab ever gets that busy.
const MAX_LEDGER_PAGES = 10;

export interface CustomerLedger {
  currentBalance: number;
  newestFirst: JournalEntry[];
}

/**
 * The customer's current ledger balance and their journals back to this transaction, read from
 * the Issuer Admin API (through the BFF). The screen rewinds them into "before" and "after".
 * Null when the card isn't known to the issuer.
 */
export function useCustomerLedger(maskedPan: string | undefined, rrn: string) {
  return useQuery({
    queryKey: ["transactions", rrn, "customer-ledger"],
    enabled: Boolean(maskedPan),
    queryFn: async (): Promise<CustomerLedger | null> => {
      const { data: cards, error } = await client.GET("/v1/cards", { fetch: liveFetch });
      if (error) throw error;
      const card = cards.find((c) => c.maskedPan === maskedPan);
      if (!card) return null;
      const detail = await client.GET("/v1/cards/{cardRef}", { params: { path: { cardRef: card.cardRef } }, fetch: liveFetch });
      if (detail.error) throw detail.error;
      return { currentBalance: detail.data.ledgerBalance.amount, newestFirst: await ledgerBackTo(card.cardRef, rrn) };
    },
  });
}

async function ledgerBackTo(cardRef: string, rrn: string): Promise<JournalEntry[]> {
  const entries: JournalEntry[] = [];
  let cursor: string | undefined;
  for (let page = 0; page < MAX_LEDGER_PAGES; page++) {
    const { data, error } = await client.GET("/v1/cards/{cardRef}/ledger", {
      params: { path: { cardRef }, query: { limit: LEDGER_PAGE_SIZE, cursor } },
      fetch: liveFetch,
    });
    if (error) throw error;
    entries.push(...data.items);
    cursor = data.nextCursor ?? undefined;
    // A transaction's journals are adjacent in time, so once past them the rest is older history.
    const seen = entries.some((e) => e.rrn === rrn);
    if (!cursor || (seen && entries[entries.length - 1].rrn !== rrn)) break;
  }
  return entries;
}
