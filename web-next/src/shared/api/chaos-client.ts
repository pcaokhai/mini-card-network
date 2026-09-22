import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import type { paths, components } from "@/shared/api/generated/schema";

export type ChaosScenario = components["schemas"]["ChaosScenario"];
export type ChaosScenarioId = ChaosScenario["id"];
export type ChaosRun = components["schemas"]["ChaosRun"];

export const CHAOS_SCENARIO_IDS: readonly ChaosScenarioId[] = [
  "SLOW_NETWORK",
  "CONNECTION_CUT",
  "DROP_RESPONSE",
  "DUPLICATE_REQUEST",
  "ISSUER_DOWN",
  "LATE_RESPONSE",
];

const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost";
const client = createClient<paths>({ baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? `${origin}/api` });

// openapi-fetch captures `globalThis.fetch` at createClient() time (module load), which runs
// before MSW's `server.listen()` patches it in tests — pass a thunk so each request re-reads
// the current global fetch (see lab-client.ts).
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const SCENARIOS_KEY = ["chaos", "scenarios"] as const;
const runKey = (runId: string) => ["chaos", "run", runId] as const;

/** WS `chaos.run.progress` is the live channel; this poll is a low-frequency backstop only. */
const RUN_POLL_INTERVAL_MS = 2000;

/**
 * Normalize the API's scenario list into the fixed six-id canonical set (Ruling in
 * docs/plans/MCN-405.md): the MSW mock can return a random subset with duplicate ids, and the
 * real gateway could in principle omit one — the screen always renders exactly six cards.
 */
function normalizeScenarios(scenarios: ChaosScenario[]): ChaosScenario[] {
  const byId = new Map(scenarios.map((s) => [s.id, s]));
  return CHAOS_SCENARIO_IDS.map(
    (id) => byId.get(id) ?? { id, enabled: false, easyText: "", technicalText: "" },
  );
}

export function useChaosScenarios() {
  return useQuery({
    queryKey: SCENARIOS_KEY,
    queryFn: async (): Promise<ChaosScenario[]> => {
      const { data, error } = await client.GET("/v1/chaos/scenarios", { fetch: liveFetch });
      if (error) throw error;
      return normalizeScenarios(data);
    },
  });
}

export function useSetChaosScenario() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { scenarioId: ChaosScenarioId; enabled: boolean }): Promise<ChaosScenario> => {
      const { data, error } = await client.PUT("/v1/chaos/scenarios/{scenarioId}", {
        params: { path: { scenarioId: vars.scenarioId }, header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { enabled: vars.enabled },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: SCENARIOS_KEY });
    },
  });
}

export function useStartChaosRun() {
  return useMutation({
    mutationFn: async (transactions: number): Promise<ChaosRun> => {
      const { data, error } = await client.POST("/v1/chaos/runs", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { transactions },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useChaosRun(runId: string | null) {
  return useQuery({
    queryKey: runKey(runId ?? "none"),
    enabled: runId != null,
    queryFn: async (): Promise<ChaosRun> => {
      const { data, error } = await client.GET("/v1/chaos/runs/{runId}", {
        params: { path: { runId: runId as string } },
        fetch: liveFetch,
      });
      if (error) throw error;
      return data;
    },
    refetchInterval: RUN_POLL_INTERVAL_MS,
  });
}
