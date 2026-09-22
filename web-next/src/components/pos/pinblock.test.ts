import { describe, expect, it } from "vitest";
import { buildPinBlock, encryptPinBlockForSimulator } from "./pinblock";

describe("buildPinBlock", () => {
  it("builds an ISO 9564-1 format-0 block of 16 hex chars", () => {
    const block = buildPinBlock("1234", "9704360000004417");
    expect(block).toMatch(/^[0-9A-F]{16}$/);
  });

  it("is deterministic for the same PIN and PAN", () => {
    expect(buildPinBlock("1234", "9704360000004417")).toEqual(buildPinBlock("1234", "9704360000004417"));
  });

  it("differs for different PINs on the same card", () => {
    expect(buildPinBlock("1234", "9704360000004417")).not.toEqual(buildPinBlock("4321", "9704360000004417"));
  });
});

describe("encryptPinBlockForSimulator", () => {
  it("returns 16 hex chars matching the API's encryptedPinBlock pattern", async () => {
    const result = await encryptPinBlockForSimulator("1234", "9704360000004417");
    expect(result).toMatch(/^[0-9A-F]{16}$/);
  });
});
