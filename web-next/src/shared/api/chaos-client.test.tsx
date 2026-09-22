import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { handlers } from "@/mocks/generated/handlers";
import { useChaosScenarios, useSetChaosScenario, useStartChaosRun, CHAOS_SCENARIO_IDS } from "@/shared/api/chaos-client";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useChaosScenarios", () => {
  it("normalizes into exactly the six canonical scenario ids __MCN_405_AC1", async () => {
    const { result } = renderHook(() => useChaosScenarios(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.map((s) => s.id)).toEqual(CHAOS_SCENARIO_IDS);
  });
});

describe("useSetChaosScenario", () => {
  it("toggles a scenario __MCN_405_AC1", async () => {
    const { result } = renderHook(() => useSetChaosScenario(), { wrapper });
    result.current.mutate({ scenarioId: "SLOW_NETWORK", enabled: true });
    await waitFor(() => expect(result.current.isSuccess || result.current.isError).toBe(true));
    if (result.current.isError) throw result.current.error;
  });
});

describe("useStartChaosRun", () => {
  it("posts transactions and returns a run __MCN_405_AC2", async () => {
    const { result } = renderHook(() => useStartChaosRun(), { wrapper });
    result.current.mutate(50);
    await waitFor(() => expect(result.current.isSuccess || result.current.isError).toBe(true));
    if (result.current.isError) throw result.current.error;
    expect(typeof result.current.data?.runId).toBe("string");
  });
});
