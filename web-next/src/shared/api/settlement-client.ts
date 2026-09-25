import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { paths, components } from "@/shared/api/generated/schema";

export type SettlementDay = components["schemas"]["SettlementDay"];
export type ReconBreak = components["schemas"]["ReconBreak"];
export type ClearingFile = components["schemas"]["ClearingFile"];
type ResolveBody = paths["/v1/settlement/breaks/{breakId}/resolutions"]["post"]["requestBody"]["content"]["application/json"];

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch reads `globalThis.fetch` at createClient() time, before MSW patches it in
// tests (see cards-client.ts's same note) — pass a thunk so each request uses the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const SETTLEMENT_KEY = ["settlement"] as const;
const dayKey = (businessDate: string) => ["settlement", "day", businessDate] as const;
const breaksKey = (businessDate: string) => ["settlement", "breaks", businessDate] as const;
/** Ruling R4: the 0500 exchange follows the cutover asynchronously, so poll until it lands. */
const TOTALS_POLL_MS = 1000;
const MAX_RETRIES = 3;

/** A non-2xx answer: the HTTP status plus the RFC 9457 problem's detail (or the raw body) as the message. */
export class SettlementApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "SettlementApiError";
  }
}

function toError(response: Response, error: unknown): SettlementApiError {
  const problem = (typeof error === "object" && error !== null ? error : {}) as { detail?: unknown; title?: unknown };
  const text = problem.detail ?? problem.title ?? (typeof error === "string" && error !== "" ? error : undefined);
  return new SettlementApiError(response.status, String(text ?? `HTTP ${response.status}`));
}

/** openapi-fetch resolves a body-less error with neither data nor error, so check the status too. */
function unwrap<T>({ data, error, response }: { data?: T; error?: unknown; response: Response }): T {
  if (!response.ok || data === undefined) throw toError(response, error);
  return data;
}

const idempotencyKey = () => ({ header: { "Idempotency-Key": crypto.randomUUID() } });

export function useSettlementDay(businessDate: string) {
  return useQuery({
    queryKey: dayKey(businessDate),
    // A 4xx (the real stack's 404 before R7) will not change on retry; show the unavailable state at once.
    retry: (failures, error) => failures < MAX_RETRIES && !(error instanceof SettlementApiError && error.status < 500),
    refetchInterval: (query) => (query.state.data?.stage === "CUTOVER_DONE" ? TOTALS_POLL_MS : false),
    queryFn: async (): Promise<SettlementDay> =>
      unwrap(await client.GET("/v1/settlement/days/{businessDate}", { params: { path: { businessDate } }, fetch: liveFetch })),
  });
}

export function useBreaks(businessDate: string, enabled: boolean) {
  return useQuery({
    queryKey: breaksKey(businessDate),
    enabled,
    queryFn: async (): Promise<ReconBreak[]> =>
      unwrap(await client.GET("/v1/settlement/days/{businessDate}/breaks", { params: { path: { businessDate } }, fetch: liveFetch })),
  });
}

function useSettlementMutation<TData, TVars = void>(mutationFn: (vars: TVars) => Promise<TData>) {
  const queryClient = useQueryClient();
  return useMutation({ mutationFn, onSuccess: () => queryClient.invalidateQueries({ queryKey: SETTLEMENT_KEY }) });
}

export function useCutover(businessDate: string) {
  return useSettlementMutation(async (): Promise<SettlementDay> =>
    unwrap(
      await client.POST("/v1/settlement/days/{businessDate}/cutover", {
        params: { path: { businessDate }, ...idempotencyKey() },
        fetch: liveFetch,
      }),
    ),
  );
}

export function useReconciliation(businessDate: string) {
  return useSettlementMutation(async (): Promise<SettlementDay> =>
    unwrap(
      await client.POST("/v1/settlement/days/{businessDate}/reconciliations", {
        params: { path: { businessDate }, ...idempotencyKey() },
        fetch: liveFetch,
      }),
    ),
  );
}

export function useResolveBreak() {
  return useSettlementMutation(async ({ breakId, ...body }: ResolveBody & { breakId: string }): Promise<ReconBreak> =>
    unwrap(
      await client.POST("/v1/settlement/breaks/{breakId}/resolutions", {
        params: { path: { breakId }, ...idempotencyKey() },
        body,
        fetch: liveFetch,
      }),
    ),
  );
}

export function useGenerateClearingFile(businessDate: string) {
  return useSettlementMutation(async (): Promise<ClearingFile> =>
    unwrap(
      await client.POST("/v1/settlement/days/{businessDate}/clearing-files", {
        params: { path: { businessDate }, ...idempotencyKey() },
        fetch: liveFetch,
      }),
    ),
  );
}
