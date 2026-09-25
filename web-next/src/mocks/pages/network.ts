import type { HttpHandler } from "msw";

/** Network operations under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const networkHandlers: HttpHandler[] = [];
