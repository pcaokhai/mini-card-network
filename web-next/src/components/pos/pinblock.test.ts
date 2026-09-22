import { describe, expect, it } from "vitest";
import { buildPinBlock, encryptPinBlockForSimulator } from "./pinblock";

// A synthetic 16-digit value, not one of the real fixture cards' PANs in
// contracts/fixtures/cards.json - this is a pure algorithm test, not a card.
const SYNTHETIC_PAN = "1111222233334444";

describe("buildPinBlock", () => {
  it("builds an ISO 9564-1 format-0 block of 16 hex chars", () => {
    const block = buildPinBlock("1234", SYNTHETIC_PAN);
    expect(block).toMatch(/^[0-9A-F]{16}$/);
  });

  it("is deterministic for the same PIN and PAN", () => {
    expect(buildPinBlock("1234", SYNTHETIC_PAN)).toEqual(buildPinBlock("1234", SYNTHETIC_PAN));
  });

  it("differs for different PINs on the same card", () => {
    expect(buildPinBlock("1234", SYNTHETIC_PAN)).not.toEqual(buildPinBlock("4321", SYNTHETIC_PAN));
  });
});

describe("encryptPinBlockForSimulator", () => {
  it("returns 16 hex chars matching the API's encryptedPinBlock pattern", async () => {
    const result = await encryptPinBlockForSimulator("1234", SYNTHETIC_PAN);
    expect(result).toMatch(/^[0-9A-F]{16}$/);
  });
});
