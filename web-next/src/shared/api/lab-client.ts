import { keepPreviousData, useQuery } from "@tanstack/react-query";
import createClient from "openapi-fetch";
import { apiBaseUrl } from "@/shared/api/base-url";
import type { components, paths } from "@/shared/api/generated/schema";

export type DecodedMessage = components["schemas"]["DecodedMessage"];
export interface SampleMessage {
  mti: string;
  label: string;
  raw: string;
}

const client = createClient<paths>({ baseUrl: apiBaseUrl() });

// openapi-fetch captures `globalThis.fetch` at createClient() time (module load), before MSW's
// `server.listen()` patches it in tests — pass a thunk so each request reads the current global fetch.
const liveFetch = (...args: Parameters<typeof globalThis.fetch>) => globalThis.fetch(...args);

/** MCN-103's sample messages: the tabs of the Message Lab. */
export function useSampleMessages() {
  return useQuery({
    queryKey: ["lab", "samples"],
    queryFn: async (): Promise<SampleMessage[]> => {
      const { data, error } = await client.GET("/v1/lab/messages/samples", { fetch: liveFetch });
      if (error) throw error;
      return data;
    },
  });
}

/** Decoding is a pure function of `raw`, so it is cached like a read; the previous message stays
 *  on screen while the next one decodes, so switching samples never blanks the page. */
export function useDecodedMessage(raw: string | undefined) {
  return useQuery({
    queryKey: ["lab", "decode", raw],
    enabled: Boolean(raw),
    staleTime: Infinity,
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<DecodedMessage> => {
      const { data, error } = await client.POST("/v1/lab/messages/decode", { body: { raw: raw ?? "" }, fetch: liveFetch });
      if (error) throw error;
      return data;
    },
  });
}
