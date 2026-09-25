import { afterEach, describe, expect, it, vi } from "vitest";
import { GET, PUT } from "./route";

const ctx = (...path: string[]) => ({ params: Promise.resolve({ path }) });

function stubUpstream(body = "{}", init: ResponseInit = { status: 200 }) {
  const fetchMock = vi.fn<typeof fetch>(async () => new Response(body, init));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe("BFF proxy (web-next/CLAUDE.md: the browser talks only to Next.js)", () => {
  it("sends card calls to the issuer admin API", async () => {
    const upstream = stubUpstream();

    await GET(new Request("http://web/api/v1/cards/crd_1/ledger?limit=50"), ctx("v1", "cards", "crd_1", "ledger"));

    expect(String(upstream.mock.calls[0][0])).toBe("http://localhost:8081/v1/cards/crd_1/ledger?limit=50");
  });

  it("sends everything else to the gateway", async () => {
    const upstream = stubUpstream();

    await GET(new Request("http://web/api/v1/transactions?status=APPROVED"), ctx("v1", "transactions"));

    expect(String(upstream.mock.calls[0][0])).toBe("http://localhost:8080/v1/transactions?status=APPROVED");
  });

  it("forwards the method, body and the headers the API relies on, and returns status and ETag", async () => {
    const upstream = stubUpstream('{"ok":true}', { status: 412, headers: { ETag: '"v2"', "Content-Type": "application/json" } });
    const request = new Request("http://web/api/v1/cards/crd_1/limits", {
      method: "PUT",
      headers: { "If-Match": '"v1"', "Idempotency-Key": "k-1", "Content-Type": "application/json", Cookie: "secret" },
      body: '{"a":1}',
    });

    const response = await PUT(request, ctx("v1", "cards", "crd_1", "limits"));

    const [, init] = upstream.mock.calls[0];
    const sent = new Headers(init?.headers);
    expect(init?.method).toBe("PUT");
    expect(sent.get("If-Match")).toBe('"v1"');
    expect(sent.get("Idempotency-Key")).toBe("k-1");
    expect(sent.get("Cookie")).toBeNull();
    expect(response.status).toBe(412);
    expect(response.headers.get("ETag")).toBe('"v2"');
    expect(await response.text()).toBe('{"ok":true}');
  });

  it("answers 502 problem+json when the upstream is down", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Promise.reject(new TypeError("fetch failed"))));

    const response = await GET(new Request("http://web/api/v1/cards"), ctx("v1", "cards"));

    expect(response.status).toBe(502);
    expect(response.headers.get("Content-Type")).toBe("application/problem+json");
  });
});
