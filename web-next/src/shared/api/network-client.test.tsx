import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { handlers } from "@/mocks/generated/handlers";
import { useLinkAction, useLinks, useNetworkEvents } from "@/shared/api/network-client";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useLinks", () => {
  it("fetches links from the generated mock __MCN_205_AC1", async () => {
    const { result } = renderHook(() => useLinks(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(Array.isArray(result.current.data)).toBe(true);
  });
});

describe("useNetworkEvents", () => {
  it("fetches the events list's items __MCN_205_AC2", async () => {
    const { result } = renderHook(() => useNetworkEvents(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(Array.isArray(result.current.data)).toBe(true);
  });
});

describe("useLinkAction", () => {
  it("posts sign-on for a link __MCN_205_AC3", async () => {
    const { result } = renderHook(() => useLinkAction(), { wrapper });
    result.current.mutate({ linkId: "issuer", action: "sign-on" });
    await waitFor(() => expect(result.current.isSuccess || result.current.isError).toBe(true));
    if (result.current.isError) throw result.current.error;
  });
});
