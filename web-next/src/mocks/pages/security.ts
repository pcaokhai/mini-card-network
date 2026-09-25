import type { HttpHandler } from "msw";

/** Security and Keys under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const securityHandlers: HttpHandler[] = [];
