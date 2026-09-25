import { useQuery } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type Overview = components["schemas"]["Overview"];

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch reads `globalThis.fetch` at createClient() time, before MSW patches it in
// tests (see network-client.ts) — pass a thunk so each request uses the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const OVERVIEW_KEY = ["metrics", "overview"] as const;

/** KPIs are point-in-time snapshots, not WS-pushed this sprint (see MCN-306 plan ruling) — poll. */
const REFETCH_INTERVAL_MS = 30_000;

export function useOverview() {
  return useQuery({
    queryKey: OVERVIEW_KEY,
    queryFn: async (): Promise<Overview> => {
      const { data, error } = await client.GET("/v1/metrics/overview", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
    refetchInterval: REFETCH_INTERVAL_MS,
  });
}

const RECENT_TRANSACTIONS_KEY = ["transactions", "recent"] as const;
const RECENT_TRANSACTIONS_LIMIT = 8;

/**
 * Seeds the live feed so the console is not blank before the first WebSocket event
 * arrives; new events are appended on top of this snapshot.
 */
export function useRecentTransactions() {
  return useQuery({
    queryKey: RECENT_TRANSACTIONS_KEY,
    queryFn: async () => {
      const { data, error } = await client.GET("/v1/transactions", {
        params: { query: { limit: RECENT_TRANSACTIONS_LIMIT } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data.items;
    },
  });
}
