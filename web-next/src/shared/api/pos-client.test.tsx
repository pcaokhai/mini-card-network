import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { handlers } from "@/mocks/generated/handlers";
import { useCreateBalanceInquiry, useCreateCompletion, useCreatePreAuth, useCreateRefund } from "./pos-client";

const server = setupServer(...handlers);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient();
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("pos-client advanced transaction hooks", () => {
  it("useCreatePreAuth posts the card-present request to /v1/transactions/pre-authorizations", async () => {
    let receivedBody: unknown;
    server.use(
      http.post("/api/v1/transactions/pre-authorizations", async ({ request }) => {
        receivedBody = await request.clone().json();
        return HttpResponse.json({ rrn: "r1", type: "PREAUTH" }, { status: 201 });
      }),
    );
    const { result } = renderHook(() => useCreatePreAuth(), { wrapper });
    result.current.mutate({
      terminalId: "00000042",
      cardToken: "tok_normal",
      entryMode: "MANUAL_PIN",
      amount: { amount: 1000, currency: "704" },
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(receivedBody).toMatchObject({ terminalId: "00000042", cardToken: "tok_normal", amount: { amount: 1000 } });
  });

  it("useCreateCompletion posts amount to the rrn-scoped completions path", async () => {
    let receivedBody: unknown;
    server.use(
      http.post("/api/v1/transactions/:rrn/completions", async ({ request, params }) => {
        const body = (await request.clone().json()) as Record<string, unknown>;
        receivedBody = { ...body, rrn: params.rrn };
        return HttpResponse.json({ rrn: params.rrn, type: "COMPLETION" }, { status: 201 });
      }),
    );
    const { result } = renderHook(() => useCreateCompletion(), { wrapper });
    result.current.mutate({ rrn: "123456789012", amount: { amount: 500, currency: "704" } });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(receivedBody).toMatchObject({ rrn: "123456789012", amount: { amount: 500 } });
  });

  it("useCreateRefund posts the card-present request to /v1/transactions/refunds", async () => {
    let receivedBody: unknown;
    server.use(
      http.post("/api/v1/transactions/refunds", async ({ request }) => {
        receivedBody = await request.clone().json();
        return HttpResponse.json({ rrn: "r1", type: "REFUND" }, { status: 201 });
      }),
    );
    const { result } = renderHook(() => useCreateRefund(), { wrapper });
    result.current.mutate({
      terminalId: "00000042",
      cardToken: "tok_normal",
      entryMode: "MANUAL_PIN",
      amount: { amount: 1000, currency: "704" },
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(receivedBody).toMatchObject({ terminalId: "00000042", amount: { amount: 1000 } });
  });

  it("useCreateBalanceInquiry posts card-present data with no amount to /v1/transactions/balance-inquiries", async () => {
    let receivedBody: unknown;
    server.use(
      http.post("/api/v1/transactions/balance-inquiries", async ({ request }) => {
        receivedBody = await request.clone().json();
        return HttpResponse.json({ rrn: "r1", type: "BALANCE" }, { status: 201 });
      }),
    );
    const { result } = renderHook(() => useCreateBalanceInquiry(), { wrapper });
    result.current.mutate({ terminalId: "00000042", cardToken: "tok_normal", entryMode: "MANUAL_PIN" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(receivedBody).toMatchObject({ terminalId: "00000042", cardToken: "tok_normal" });
    expect(receivedBody).not.toHaveProperty("amount");
  });
});
