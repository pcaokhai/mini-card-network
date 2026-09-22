import { useQuery } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import type { paths, components } from "@/shared/api/generated/schema";

export type Overview = components["schemas"]["Overview"];

const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost";
const client = createClient<paths>({ baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? `${origin}/api` });

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
