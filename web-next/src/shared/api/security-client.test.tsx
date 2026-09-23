import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { handlers } from "@/mocks/generated/handlers";
import { useAcquirerKeys, useIssuerKeys } from "./security-client";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("security-client", () => {
  it("fetches acquirer keys via GET /v1/keys/acquirer__MCN_505_AC1", async () => {
    const { result } = renderHook(() => useAcquirerKeys(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(Array.isArray(result.current.data)).toBe(true);
  });

  it("fetches issuer keys via GET /v1/keys/issuer__MCN_505_AC1", async () => {
    const { result } = renderHook(() => useIssuerKeys(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(Array.isArray(result.current.data)).toBe(true);
  });
});
