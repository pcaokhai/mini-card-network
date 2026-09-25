import { useMutation } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type Transaction = components["schemas"]["Transaction"];
export type EntryMode = components["schemas"]["EntryMode"];

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

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

export type PreAuthVars = components["schemas"]["PurchaseRequest"];
export type RefundVars = components["schemas"]["PurchaseRequest"];
export type BalanceInquiryVars = components["schemas"]["CardPresentData"];
export interface CompletionVars {
  rrn: string;
  amount: components["schemas"]["Money"];
}

export function useCreatePreAuth() {
  return useMutation({
    mutationFn: async (vars: PreAuthVars): Promise<Transaction> => {
      const { data, error } = await client.POST("/v1/transactions/pre-authorizations", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: vars,
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useCreateCompletion() {
  return useMutation({
    mutationFn: async ({ rrn, amount }: CompletionVars): Promise<Transaction> => {
      const { data, error } = await client.POST("/v1/transactions/{rrn}/completions", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() }, path: { rrn } },
        body: { amount },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useCreateRefund() {
  return useMutation({
    mutationFn: async (vars: RefundVars): Promise<Transaction> => {
      const { data, error } = await client.POST("/v1/transactions/refunds", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: vars,
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useCreateBalanceInquiry() {
  return useMutation({
    mutationFn: async (vars: BalanceInquiryVars): Promise<Transaction> => {
      const { data, error } = await client.POST("/v1/transactions/balance-inquiries", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: vars,
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}
