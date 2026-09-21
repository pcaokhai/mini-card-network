"use client";

import { type ReactNode, useEffect, useState } from "react";

const MOCKS_ENABLED = process.env.NEXT_PUBLIC_API_MOCKS === "true";

/** MCN-004-AC5: starts the MSW worker (handlers generated from contracts/openapi.yaml) before rendering children. */
export function MockProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(!MOCKS_ENABLED);
  useEffect(() => {
    if (!MOCKS_ENABLED) return;
    void import("@/mocks/generated/browser").then(({ worker }) =>
      worker.start({ onUnhandledRequest: "bypass" }).then(() => setReady(true)),
    );
  }, []);
  return ready ? children : null;
}
