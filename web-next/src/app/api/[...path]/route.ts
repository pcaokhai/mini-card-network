// BFF (web-next/CLAUDE.md): the browser calls /api/v1/..., and this server-side handler forwards to
// the service that owns the path, so the browser never talks to the gateway or the Issuer Admin API
// directly. Cards, accounts and the issuer key inventory belong to the issuer (docs/04 §1);
// everything else to the gateway.
// ponytail: a pass-through proxy, no auth or caching; add those when the BFF gains sessions.

const GATEWAY_URL = process.env.GATEWAY_URL ?? "http://localhost:8080";
const ISSUER_ADMIN_URL = process.env.ISSUER_ADMIN_URL ?? "http://localhost:8081";

const FORWARDED_REQUEST_HEADERS = ["accept", "content-type", "idempotency-key", "if-match", "traceparent"];
const FORWARDED_RESPONSE_HEADERS = ["content-type", "etag", "location", "retry-after"];

type Context = { params: Promise<{ path: string[] }> };

function upstreamFor(path: string[]): string {
  if (path[0] !== "v1") return GATEWAY_URL;
  const issuerOwned = path[1] === "cards" || path[1] === "accounts" || (path[1] === "keys" && path[2] === "issuer");
  return issuerOwned ? ISSUER_ADMIN_URL : GATEWAY_URL;
}

function pick(headers: Headers, names: string[]): Headers {
  const picked = new Headers();
  for (const name of names) {
    const value = headers.get(name);
    if (value !== null) picked.set(name, value);
  }
  return picked;
}

function badGateway(detail: string): Response {
  return Response.json(
    { type: "https://mcn.local/problems/upstream-unavailable", title: "Upstream unavailable", status: 502, detail },
    { status: 502, headers: { "Content-Type": "application/problem+json" } },
  );
}

async function proxy(request: Request, { params }: Context): Promise<Response> {
  const { path } = await params;
  const target = `${upstreamFor(path)}/${path.map(encodeURIComponent).join("/")}${new URL(request.url).search}`;
  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  try {
    const upstream = await fetch(target, {
      method: request.method,
      headers: pick(request.headers, FORWARDED_REQUEST_HEADERS),
      body: hasBody ? await request.text() : undefined,
      cache: "no-store",
    });
    return new Response(upstream.body, {
      status: upstream.status,
      headers: pick(upstream.headers, FORWARDED_RESPONSE_HEADERS),
    });
  } catch (error) {
    return badGateway(error instanceof Error ? error.message : "upstream request failed");
  }
}

export { proxy as GET, proxy as POST, proxy as PUT, proxy as PATCH, proxy as DELETE };
