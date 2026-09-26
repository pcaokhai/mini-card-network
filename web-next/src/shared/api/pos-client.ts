import { useMutation } from "@tanstack/react-query";
import { useRef } from "react";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type Transaction = components["schemas"]["Transaction"];
export type EntryMode = components["schemas"]["EntryMode"];

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// See network-client.ts: pass a thunk so requests use the current global fetch (MSW patches it in tests).
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

/**
 * A mutation whose Idempotency-Key belongs to the draft, not the click (POS-G14): retrying the same
 * draft after a failure whose outcome is unknown (500, 502, a dropped connection) resends the same
 * key, so the gateway replays instead of charging twice. A changed draft or a success mints a new key.
 */
function useDraftMutation<V, R>(send: (vars: V, idempotencyKey: string) => Promise<R>) {
  const last = useRef<{ draft: string; key: string } | null>(null);
  return useMutation({
    mutationFn: (vars: V) => {
      const draft = JSON.stringify(vars);
      if (last.current?.draft !== draft) last.current = { draft, key: crypto.randomUUID() };
      return send(vars, last.current.key);
    },
    onSuccess: () => {
      last.current = null;
    },
  });
}

export type CreatePurchaseVars = components["schemas"]["PurchaseRequest"];

export function useCreatePurchase() {
  return useDraftMutation(async (vars: CreatePurchaseVars, idempotencyKey: string): Promise<Transaction> => {
    const { data, error } = await client.POST("/v1/transactions/purchases", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: vars,
      fetch: liveFetch,
    });
    if (error) throw error;
    return data;
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
  return useDraftMutation(async (vars: PreAuthVars, idempotencyKey: string): Promise<Transaction> => {
    const { data, error } = await client.POST("/v1/transactions/pre-authorizations", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: vars,
      fetch: liveFetch,
    });
    if (error) throw error;
    return data;
  });
}

export function useCreateCompletion() {
  return useDraftMutation(async ({ rrn, amount }: CompletionVars, idempotencyKey: string): Promise<Transaction> => {
    const { data, error } = await client.POST("/v1/transactions/{rrn}/completions", {
      params: { header: { "Idempotency-Key": idempotencyKey }, path: { rrn } },
      body: { amount },
      fetch: liveFetch,
    });
    if (error) throw error;
    return data;
  });
}

export function useCreateRefund() {
  return useDraftMutation(async (vars: RefundVars, idempotencyKey: string): Promise<Transaction> => {
    const { data, error } = await client.POST("/v1/transactions/refunds", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: vars,
      fetch: liveFetch,
    });
    if (error) throw error;
    return data;
  });
}

export function useCreateBalanceInquiry() {
  return useDraftMutation(async (vars: BalanceInquiryVars, idempotencyKey: string): Promise<Transaction> => {
    const { data, error } = await client.POST("/v1/transactions/balance-inquiries", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: vars,
      fetch: liveFetch,
    });
    if (error) throw error;
    return data;
  });
}
