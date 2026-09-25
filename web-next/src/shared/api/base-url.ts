const MOCKS_ENABLED = process.env.NEXT_PUBLIC_API_MOCKS === "true";

/**
 * MSW registers its handlers under `${origin}/api`, so mock mode must ignore
 * NEXT_PUBLIC_API_BASE_URL — otherwise .env.local's real-gateway URL wins and the
 * worker never intercepts, leaving every screen empty under `pnpm dev:mock`.
 */
export function apiBaseUrl(): string {
  const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost";
  if (MOCKS_ENABLED) return `${origin}/api`;
  return process.env.NEXT_PUBLIC_API_BASE_URL ?? `${origin}/api`;
}
