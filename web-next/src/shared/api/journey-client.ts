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
