import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { setupServer } from "msw/node";
import type { ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { createSettlementHandlers } from "@/mocks/pages/settlement";
import { SettlementApiError, useBreaks, useGenerateClearingFile, useResolveBreak, useSettlementDay } from "./settlement-client";

const server = setupServer();
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("settlement-client", () => {
  it("fetches the settlement day by business date __MCN_705_AC1", async () => {
    server.use(...createSettlementHandlers("TOTALS_EXCHANGED"));
    const { result } = renderHook(() => useSettlementDay("2026-09-21"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.stage).toBe("TOTALS_EXCHANGED");
    expect(result.current.data?.totals).toHaveLength(5);
  });

  it("lists breaks only once they are wanted __MCN_705_AC1", async () => {
    server.use(...createSettlementHandlers("RECONCILED"));
    const { result } = renderHook(() => useBreaks("2026-09-21", true), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.map((b) => b.breakType)).toEqual(["MISSING_AT_ISSUER", "STATUS_MISMATCH", "AMOUNT_MISMATCH"]);
  });

  it("posts a resolution with a note __MCN_705_AC1", async () => {
    server.use(...createSettlementHandlers("RECONCILED"));
    const { result } = renderHook(() => useResolveBreak(), { wrapper });
    await act(() => result.current.mutateAsync({ breakId: "brk-626514000098", resolution: "MANUAL_ADJUSTED", note: "adjust" }));
    await waitFor(() => expect(result.current.data?.resolution).toBe("MANUAL_ADJUSTED"));
  });

  it("surfaces the 409 open-breaks problem with its status and detail __MCN_705_AC2", async () => {
    server.use(...createSettlementHandlers("RECONCILED"));
    const { result } = renderHook(() => useGenerateClearingFile("2026-09-21"), { wrapper });
    await act(() => result.current.mutateAsync().catch(() => undefined));
    await waitFor(() => expect(result.current.error).toBeInstanceOf(SettlementApiError));
    expect(result.current.error).toMatchObject({ status: 409, message: "Còn 3 chênh lệch chưa xử lý. Hãy xử lý hết trước khi xuất file quyết toán." });
  });
});
