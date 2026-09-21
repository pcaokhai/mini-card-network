import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { handlers } from "@/mocks/generated/handlers";

// Vitest runs from web-next/, so the contract is one level up.
const spec = readFileSync(resolve(process.cwd(), "../contracts/openapi.yaml"), "utf8");

describe("generated MSW handlers", () => {
  it("cover every operation in contracts/openapi.yaml __MCN_004_AC5", () => {
    const operations = spec.match(/^\s+operationId:/gm) ?? [];
    expect(handlers.length).toBe(operations.length);
  });
});
