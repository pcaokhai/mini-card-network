import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type Link = components["schemas"]["Link"];
export type NetworkEvent = components["schemas"]["NetworkEvent"];
export type LinkAction = "echo" | "sign-on" | "sign-off";

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch reads `globalThis.fetch` at createClient() time, before MSW patches it in
// tests (see lab-client.ts) — pass a thunk so each request uses the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const LINKS_KEY = ["network", "links"] as const;
const EVENTS_KEY = ["network", "events"] as const;

/** MCN-205: no live WS push exists yet (useWsEvents is not built) — poll instead. */
const POLL_INTERVAL_MS = 5000;

export function useLinks() {
  return useQuery({
    queryKey: LINKS_KEY,
    queryFn: async (): Promise<Link[]> => {
      const { data, error } = await client.GET("/v1/network/links", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
    refetchInterval: POLL_INTERVAL_MS,
  });
}

export function useNetworkEvents() {
  return useQuery({
    queryKey: EVENTS_KEY,
    queryFn: async (): Promise<NetworkEvent[]> => {
      const { data, error } = await client.GET("/v1/network/events", { fetch: liveFetch });
      if (error) throw error;
      return data.items;
    },
    refetchInterval: POLL_INTERVAL_MS,
  });
}

async function postLinkAction(linkId: string, action: LinkAction) {
  const opts = {
    params: { path: { linkId }, header: { "Idempotency-Key": crypto.randomUUID() } },
    fetch: liveFetch,
  };
  switch (action) {
    case "echo":
      return client.POST("/v1/network/links/{linkId}/echo", opts);
    case "sign-on":
      return client.POST("/v1/network/links/{linkId}/sign-on", opts);
    case "sign-off":
      return client.POST("/v1/network/links/{linkId}/sign-off", opts);
  }
}

const OPTIMISTIC_STATUS: Partial<Record<LinkAction, Link["status"]>> = {
  "sign-on": "SIGNED_ON",
  "sign-off": "DISCONNECTED",
};

export function useLinkAction() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { linkId: string; action: LinkAction }) => {
      const { data, error } = await postLinkAction(vars.linkId, vars.action);
      if (error) throw error;
      return data;
    },
    onMutate: async (vars) => {
      const nextStatus = OPTIMISTIC_STATUS[vars.action];
      if (!nextStatus) return undefined;
      await queryClient.cancelQueries({ queryKey: LINKS_KEY });
      const previousLinks = queryClient.getQueryData<Link[]>(LINKS_KEY);
      queryClient.setQueryData<Link[]>(LINKS_KEY, (links) =>
        links?.map((link) => (link.linkId === vars.linkId ? { ...link, status: nextStatus } : link)),
      );
      return { previousLinks };
    },
    onError: (_error, _vars, context) => {
      if (context?.previousLinks) queryClient.setQueryData(LINKS_KEY, context.previousLinks);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: LINKS_KEY });
    },
  });
}
