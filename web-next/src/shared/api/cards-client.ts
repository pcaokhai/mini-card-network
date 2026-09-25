import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type CardSummary = components["schemas"]["CardSummary"];
export type CardDetail = components["schemas"]["CardDetail"];
export type CardLimits = components["schemas"]["CardLimits"];
export type JournalEntry = components["schemas"]["JournalEntry"];
export type BlockReason = "CUSTOMER_REQUEST" | "LOST" | "STOLEN" | "FRAUD_SUSPECTED";

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch reads `globalThis.fetch` at createClient() time, before MSW patches it in
// tests (see network-client.ts) — pass a thunk so each request uses the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const CARDS_KEY = ["cards"] as const;
const cardKey = (cardRef: string) => ["cards", cardRef] as const;
const ledgerKey = (cardRef: string) => ["cards", cardRef, "ledger"] as const;

export function useCards() {
  return useQuery({
    queryKey: CARDS_KEY,
    queryFn: async (): Promise<CardSummary[]> => {
      const { data, error } = await client.GET("/v1/cards", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
  });
}

/** ETag lives on the raw Response (openapi-fetch's typed result exposes it via `response`); captured for If-Match on the next mutation. */
export function useCard(cardRef: string) {
  return useQuery({
    queryKey: cardKey(cardRef),
    queryFn: async (): Promise<{ card: CardDetail; etag: string | null }> => {
      const { data, error, response } = await client.GET("/v1/cards/{cardRef}", {
        params: { path: { cardRef } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return { card: data, etag: response.headers.get("ETag") };
    },
  });
}

export function useCardLedger(cardRef: string) {
  return useQuery({
    queryKey: ledgerKey(cardRef),
    queryFn: async (): Promise<JournalEntry[]> => {
      const { data, error } = await client.GET("/v1/cards/{cardRef}/ledger", {
        params: { path: { cardRef } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data.items;
    },
  });
}

export class PreconditionFailedError extends Error {}

export function useUpdateLimits(cardRef: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { limits: CardLimits; ifMatch: string | null }): Promise<CardDetail> => {
      const { data, error, response } = await client.PUT("/v1/cards/{cardRef}/limits", {
        params: {
          path: { cardRef },
          header: { "If-Match": vars.ifMatch ?? "", "Idempotency-Key": crypto.randomUUID() },
        },
        body: vars.limits,
        fetch: liveFetch,
      });
      if (response.status === 412) throw new PreconditionFailedError();
      if (error) throw error;
      return data;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: cardKey(cardRef) }),
  });
}

export function useBlockCard(cardRef: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (reason: BlockReason): Promise<CardDetail> => {
      const { data, error } = await client.POST("/v1/cards/{cardRef}/blocks", {
        params: { path: { cardRef }, header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { reason },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: cardKey(cardRef) }),
  });
}

export function useUnblockCard(cardRef: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (): Promise<CardDetail> => {
      const { data, error } = await client.DELETE("/v1/cards/{cardRef}/blocks", {
        params: { path: { cardRef }, header: { "Idempotency-Key": crypto.randomUUID() } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: cardKey(cardRef) }),
  });
}
