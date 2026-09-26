import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
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

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch captures `globalThis.fetch` at createClient() time (module load), which runs
// before MSW's `server.listen()` patches it in tests — pass a thunk so each request re-reads
// the current global fetch (see lab-client.ts).
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

const SCENARIOS_KEY = ["chaos", "scenarios"] as const;
/** Query key of one run, so a WS `chaos.run.progress` snapshot can land in the same cache entry. */
export const chaosRunKey = (runId: string) => ["chaos", "run", runId] as const;

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
    mutationFn: (vars: { scenarioId: ChaosScenarioId; enabled: boolean }) => putScenario(vars.scenarioId, vars.enabled),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: SCENARIOS_KEY });
    },
  });
}

async function putScenario(scenarioId: ChaosScenarioId, enabled: boolean): Promise<ChaosScenario> {
  const { data, error } = await client.PUT("/v1/chaos/scenarios/{scenarioId}", {
    params: { path: { scenarioId }, header: { "Idempotency-Key": crypto.randomUUID() } },
    body: { enabled },
    fetch: liveFetch,
  });
  if (error) throw error;
  return data;
}

/** "Tắt tất cả": one PUT enabled=false per scenario that is on. */
export function useDisableAllChaosScenarios() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (scenarioIds: ChaosScenarioId[]) => Promise.all(scenarioIds.map((id) => putScenario(id, false))),
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

/** The gateway no longer knows the run (runs live in its memory, so a restart forgets them). */
export class ChaosRunGoneError extends Error {
  constructor(runId: string) {
    super(`chaos run ${runId} not found`);
    this.name = "ChaosRunGoneError";
  }
}

export function useChaosRun(runId: string | null) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: chaosRunKey(runId ?? "none"),
    enabled: runId != null,
    queryFn: async (): Promise<ChaosRun> => {
      const id = runId as string; // enabled only when runId is set
      const { data, error, response } = await client.GET("/v1/chaos/runs/{runId}", {
        params: { path: { runId: id } },
        fetch: liveFetch,
      });
      if (response.status === 404) throw new ChaosRunGoneError(id);
      if (error) throw error;
      // A WS snapshot may already be newer than this poll (CHA-G13).
      return newerRun(queryClient.getQueryData<ChaosRun>(chaosRunKey(id)), data);
    },
    // A gone run never comes back: no retries and no more polling (CHA-G7).
    retry: (failureCount, error) => !(error instanceof ChaosRunGoneError) && failureCount < 3,
    refetchInterval: (query) =>
      isRunFinished(query.state.data) || query.state.error instanceof ChaosRunGoneError ? false : RUN_POLL_INTERVAL_MS,
  });
}

/** The later of two snapshots of one run by `seq`, so a delayed WS event can't roll the panel
 * back past a newer poll (CHA-G13). A snapshot without `seq` never wins over one with it. */
export function newerRun(cached: ChaosRun | undefined, incoming: ChaosRun): ChaosRun {
  if (cached?.seq === undefined) return incoming;
  if (incoming.seq === undefined) return cached;
  return incoming.seq >= cached.seq ? incoming : cached;
}

export function isRunFinished(run: ChaosRun | undefined): boolean {
  return run?.status === "PASSED" || run?.status === "FAILED";
}
