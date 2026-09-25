import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { labHandlers } from "@/mocks/pages/lab";
import { useDecodedMessage, useSampleMessages } from "@/shared/api/lab-client";

const server = setupServer(...labHandlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("lab-client", () => {
  it("lists the four canvas samples and decodes one __MCN_104_AC1", async () => {
    const samples = renderHook(() => useSampleMessages(), { wrapper });
    await waitFor(() => expect(samples.result.current.isSuccess).toBe(true));
    expect(samples.result.current.data?.map((s) => s.mti)).toEqual(["0200", "0210", "0420", "0800"]);

    const echo = samples.result.current.data?.[3]?.raw;
    const decoded = renderHook(() => useDecodedMessage(echo), { wrapper });
    await waitFor(() => expect(decoded.result.current.isSuccess).toBe(true));
    expect(decoded.result.current.data).toMatchObject({ mti: "0800", primaryBitmap: "8220000000000000", secondaryBitmap: "0400000000000000" });
  });
});
