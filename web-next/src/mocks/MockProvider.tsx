"use client";

import { type ReactNode, useEffect, useState } from "react";

const MOCKS_ENABLED = process.env.NEXT_PUBLIC_API_MOCKS === "true";

/** MCN-004-AC5: starts the MSW worker (handlers generated from contracts/openapi.yaml) before rendering children. */
export function MockProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(!MOCKS_ENABLED);
  useEffect(() => {
    if (!MOCKS_ENABLED) return;
    // Scenario handlers go first so they win over the faker-generated ones, which
    // answer with schema-shaped noise rather than the design canvas's numbers.
    void Promise.all([
      import("msw/browser"),
      import("@/mocks/pages"),
      import("@/mocks/journey-handlers"),
      import("@/mocks/scenario-handlers"),
      import("@/mocks/generated/handlers"),
    ]).then(([{ setupWorker }, { pageHandlers }, { journeyHandlers }, { scenarioHandlers }, { handlers }]) =>
      setupWorker(...pageHandlers, ...journeyHandlers, ...scenarioHandlers, ...handlers)
        .start({ onUnhandledRequest: "bypass" })
        .then(() => setReady(true)),
    );
  }, []);
  return ready ? children : null;
}
