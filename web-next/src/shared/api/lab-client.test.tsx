import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { handlers } from "@/mocks/generated/handlers";
import { useDecodeMessage } from "@/shared/api/lab-client";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useDecodeMessage", () => {
  it("decodes a raw message via the generated client __MCN_104_AC1", async () => {
    const { result } = renderHook(() => useDecodeMessage(), { wrapper });
    result.current.mutate({ raw: "0800822000000000000004000000000000000921073300000200301" });
    await waitFor(() => expect(result.current.isSuccess || result.current.isError).toBe(true));
    if (result.current.isError) throw result.current.error;
    // The generated MSW mock (msw-auto-mock) returns faker-random field values, not a real
    // decode of `raw`, so we assert shape/success here rather than a specific mti — a real
    // decode is exercised end-to-end once MCN-103's gateway is wired up (NEXT_PUBLIC_API_MOCKS=false).
    expect(typeof result.current.data?.mti).toBe("string");
  });
});
