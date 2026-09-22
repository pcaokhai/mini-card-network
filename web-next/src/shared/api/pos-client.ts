import { useMutation } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import type { paths, components } from "@/shared/api/generated/schema";

export type Transaction = components["schemas"]["Transaction"];
export type EntryMode = components["schemas"]["EntryMode"];

const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost";
const client = createClient<paths>({ baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? `${origin}/api` });

// See network-client.ts: pass a thunk so requests use the current global fetch (MSW patches it in tests).
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

export interface CreatePurchaseVars {
  terminalId: string;
  cardToken: string;
  entryMode: EntryMode;
  encryptedPinBlock: string;
  amount: { amount: number; currency: string };
}

export function useCreatePurchase() {
  return useMutation({
    mutationFn: async (vars: CreatePurchaseVars): Promise<Transaction> => {
      const { data, error } = await client.POST("/v1/transactions/purchases", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: vars,
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}
