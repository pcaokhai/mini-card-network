"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";

/** No QueryClientProvider existed anywhere in the app (MCN-104's useDecodeMessage only
 * worked in tests, which each wrap their own) — one singleton client for the whole tree. */
export function QueryProvider({ children }: { children: ReactNode }) {
  const [client] = useState(() => new QueryClient());
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
