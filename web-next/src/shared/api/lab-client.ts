import { useMutation } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import type { paths } from "@/shared/api/generated/schema";
import type { DecodedMessage } from "@/shared/state/message-lab";

// openapi-fetch builds a `new URL(...)` internally, which needs an absolute base
// (a bare "/api" throws "Invalid URL" in Node/jsdom); resolve against the current
// origin in the browser/jsdom, falling back to a placeholder origin during SSR/build
// where this client is never actually called (the screen is a "use client" component).
const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost";
const client = createClient<paths>({ baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL ?? `${origin}/api` });

/** MCN-103's Lab API; served by MSW in mock mode, the real gateway once wired up. */
export function useDecodeMessage() {
  return useMutation({
    mutationFn: async (body: { raw: string }): Promise<DecodedMessage> => {
      // openapi-fetch captures `globalThis.fetch` as a default at createClient() time (module
      // load), which runs before MSW's `server.listen()` patches it in tests — pass a thunk so
      // each request re-reads the *current* global fetch instead of the stale pre-patch one.
      const { data, error } = await client.POST("/v1/lab/messages/decode", {
        body,
        fetch: (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args),
      });
      if (error) throw error;
      return data as DecodedMessage;
    },
  });
}
