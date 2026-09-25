import { describe, expect, it } from "vitest";
import type { KeyInfo, KeyRotation } from "@/shared/api/security-client";
import { ILLUSTRATION_PAN, isValidPin, keyRows, pinBlockRows, rotationSteps } from "./security-model";

const key = (keyType: KeyInfo["keyType"], daysRemaining: number, lifetimeDays: number, status: KeyInfo["status"] = "ACTIVE"): KeyInfo => ({
  keyType,
  kcv: "ABC123",
  status,
  daysRemaining,
  lifetimeDays,
});

describe("keyRows MCN-505-AC1", () => {
  it("orders keys as the canvas does, from the master key down", () => {
    expect(keyRows([key("PVK", 1, 1), key("ZAK", 1, 1), key("ZMK", 1, 1), key("ZPK", 1, 1)]).map((r) => r.keyType)).toEqual([
      "ZMK",
      "ZPK",
      "ZAK",
      "PVK",
    ]);
  });

  it("flags an active key with under 20% of its lifetime left as rotate-soon MCN-505-AC1", () => {
    const [zpk, zak] = keyRows([key("ZAK", 5, 30), key("ZPK", 26, 30)]);
    expect(zpk).toMatchObject({ percent: 87, status: "active", tone: "ok" });
    expect(zak).toMatchObject({ percent: 17, status: "rotateSoon", tone: "warn" });
  });

  it("shows pending and retired keys neutrally, and clamps the bar", () => {
    const [zmk, zpk] = keyRows([key("ZMK", 400, 365, "PENDING"), key("ZPK", -3, 30, "RETIRED")]);
    expect(zmk).toMatchObject({ percent: 100, status: "pending", tone: "info" });
    expect(zpk).toMatchObject({ percent: 0, status: "retired", tone: "info" });
  });
});

describe("pinBlockRows MCN-505-AC2", () => {
  it("builds ISO 9564 format 0 from the illustration card, never showing the PIN digits", () => {
    const rows = pinBlockRows("1234", "3F9A21");
    expect(rows.map((r) => r.id)).toEqual(["pinField", "panField", "clear", "underTpk", "underZpk"]);
    expect(rows[0]?.value).toBe("04•• ••FF FFFF FFFF");
    expect(rows[1]?.value).toBe("0000 4361 2345 6789");
    // 041234FFFFFFFFFF xor 0000436123456789, worked by hand.
    expect(rows[2]?.value).toBe("0412 779E DCBA 9876");
  });

  it("uses an uppercase hex length nibble for a 12-digit PIN", () => {
    expect(pinBlockRows("123456789012", "3F9A21")[0]?.value).toBe("0C•• •••• •••• ••FF");
  });

  it("changes the ZPK row when the ZPK is rotated, but not the TPK row", () => {
    const before = pinBlockRows("1234", "3F9A21");
    const after = pinBlockRows("1234", "7D02B1");
    expect(after[3]?.value).toBe(before[3]?.value);
    expect(after[4]?.value).not.toBe(before[4]?.value);
    expect(after[4]?.value).toMatch(/^[0-9A-F]{4}( [0-9A-F]{4}){3}$/);
  });

  it("uses a made-up card that is not one of the test fixtures", () => {
    expect(ILLUSTRATION_PAN).not.toBe("9704360000004417");
    expect(ILLUSTRATION_PAN.startsWith("970436")).toBe(true);
  });

  it("accepts 4 to 12 digits only", () => {
    expect(["1234", "123456789012"].map(isValidPin)).toEqual([true, true]);
    expect(["123", "1234567890123", "12a4", ""].map(isValidPin)).toEqual([false, false, false, false]);
  });
});

describe("rotationSteps MCN-505-AC1", () => {
  it("lists the four steps as pending before any rotation", () => {
    expect(rotationSteps(undefined)).toEqual([
      { name: "GENERATE", status: "PENDING" },
      { name: "SEND_0800_161", status: "PENDING" },
      { name: "PARTNER_CONFIRM", status: "PENDING" },
      { name: "ACTIVATE", status: "PENDING" },
    ]);
  });

  it("takes each step's status from the rotation resource", () => {
    const rotation: KeyRotation = {
      rotationId: "rot_1",
      keyType: "ZPK",
      status: "RUNNING",
      newKcv: null,
      steps: [
        { name: "SEND_0800_161", status: "DONE" },
        { name: "GENERATE", status: "DONE" },
      ],
    };
    expect(rotationSteps(rotation).map((s) => s.status)).toEqual(["DONE", "DONE", "PENDING", "PENDING"]);
  });
});
