import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type KeyInfo = components["schemas"]["KeyInfo"];
export type KeyRotation = components["schemas"]["KeyRotation"];
export type RotatableKeyType = "ZPK" | "ZAK";

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch reads `globalThis.fetch` at createClient() time, before MSW patches it in
// tests (see cards-client.ts's same note) — pass a thunk so each request uses the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const ACQUIRER_KEYS_KEY = ["security", "keys", "acquirer"] as const;
const ISSUER_KEYS_KEY = ["security", "keys", "issuer"] as const;
const rotationKey = (rotationId: string) => ["security", "rotations", rotationId] as const;

export function useAcquirerKeys() {
  return useQuery({
    queryKey: ACQUIRER_KEYS_KEY,
    queryFn: async (): Promise<KeyInfo[]> => {
      const { data, error } = await client.GET("/v1/keys/acquirer", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
  });
}

export function useIssuerKeys() {
  return useQuery({
    queryKey: ISSUER_KEYS_KEY,
    queryFn: async (): Promise<KeyInfo[]> => {
      const { data, error } = await client.GET("/v1/keys/issuer", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
  });
}

export function useStartRotation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (keyType: RotatableKeyType): Promise<KeyRotation> => {
      const { data, error } = await client.POST("/v1/keys/acquirer/rotations", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { keyType },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ACQUIRER_KEYS_KEY }),
  });
}

export function useRotation(rotationId: string | null) {
  return useQuery({
    queryKey: rotationKey(rotationId ?? "none"),
    enabled: rotationId !== null,
    refetchInterval: (query) => (query.state.data?.status === "RUNNING" ? 1000 : false),
    queryFn: async (): Promise<KeyRotation> => {
      const { data, error } = await client.GET("/v1/keys/acquirer/rotations/{rotationId}", {
        params: { path: { rotationId: rotationId as string } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}
