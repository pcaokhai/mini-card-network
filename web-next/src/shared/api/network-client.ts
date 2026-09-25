import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import { useWsEvents } from "@/shared/ws/useWsEvents";
import type { paths, components } from "@/shared/api/generated/schema";

export type Link = components["schemas"]["Link"];
export type NetworkEvent = components["schemas"]["NetworkEvent"];
export type LinkAction = "echo" | "sign-on" | "sign-off";
export type SwitchStatus = components["schemas"]["SwitchStatus"];
export type Terminal = components["schemas"]["Terminal"];
export type SafQueue = paths["/v1/network/saf"]["get"]["responses"][200]["content"]["application/json"];

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch reads `globalThis.fetch` at createClient() time, before MSW patches it in
// tests (see lab-client.ts) — pass a thunk so each request uses the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const LINKS_KEY = ["network", "links"] as const;
const EVENTS_KEY = ["network", "events"] as const;
const SAF_KEY = ["network", "saf"] as const;
const SWITCH_KEY = ["network", "switch"] as const;
const TERMINALS_KEY = ["network", "terminals"] as const;

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

export function useSafQueue() {
  return useQuery({
    queryKey: SAF_KEY,
    queryFn: async (): Promise<SafQueue> => {
      const { data, error } = await client.GET("/v1/network/saf", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
    refetchInterval: POLL_INTERVAL_MS,
  });
}

export function useSwitchStatus() {
  return useQuery({
    queryKey: SWITCH_KEY,
    queryFn: async (): Promise<SwitchStatus> => {
      const { data, error } = await client.GET("/v1/network/switch", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
    refetchInterval: POLL_INTERVAL_MS,
  });
}

/** Registered terminals, counted on the Network topology's POS node. */
export function useTerminals() {
  return useQuery({
    queryKey: TERMINALS_KEY,
    queryFn: async (): Promise<Terminal[]> => {
      const { data, error } = await client.GET("/v1/terminals", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
  });
}

const LIVE_KEYS: Record<string, readonly string[]> = {
  "link.status": LINKS_KEY,
  "network.event": EVENTS_KEY,
  "saf.changed": SAF_KEY,
  "switch.status": SWITCH_KEY,
};
const LIVE_TYPES = Object.keys(LIVE_KEYS);

/** MCN-205-AC3: WS pushes refetch the matching query at once; the poll stays as a backstop. */
export function useNetworkLiveUpdates() {
  const queryClient = useQueryClient();
  useWsEvents(LIVE_TYPES, (event) => {
    const queryKey = LIVE_KEYS[event.type];
    if (queryKey) void queryClient.invalidateQueries({ queryKey });
  });
}

/** Refetch every network query, e.g. after a chaos scenario changed the network under the screen. */
export function useRefreshNetwork() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: ["network"] });
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
