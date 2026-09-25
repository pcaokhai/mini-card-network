import { buildPinBlock } from "@/components/pos/pinblock";
import type { KeyInfo, KeyRotation } from "@/shared/api/security-client";

type KeyType = KeyInfo["keyType"];
type StepName = KeyRotation["steps"][number]["name"];
type StepStatus = KeyRotation["steps"][number]["status"];

// The canvas lists the master key first, then the keys it wraps.
const KEY_ORDER: readonly KeyType[] = ["ZMK", "ZPK", "ZAK", "TPK", "TAK", "CVK", "PVK"];
const ROTATE_SOON_PERCENT = 20;

export type KeyStatusKind = "active" | "rotateSoon" | "pending" | "retired";
/** A key as the table shows it; `status` folds lifetime into ACTIVE (due for rotation or not). */
export type KeyRow = Omit<KeyInfo, "status"> & { percent: number; status: KeyStatusKind; tone: "ok" | "warn" | "info" };

function statusOf(key: KeyInfo, percent: number): KeyStatusKind {
  if (key.status === "PENDING") return "pending";
  if (key.status === "RETIRED") return "retired";
  return percent < ROTATE_SOON_PERCENT ? "rotateSoon" : "active";
}

const TONE: Record<KeyStatusKind, KeyRow["tone"]> = { active: "ok", rotateSoon: "warn", pending: "info", retired: "info" };

export function keyRows(keys: readonly KeyInfo[]): KeyRow[] {
  return [...keys]
    .sort((a, b) => KEY_ORDER.indexOf(a.keyType) - KEY_ORDER.indexOf(b.keyType))
    .map((key) => {
      const ratio = key.lifetimeDays > 0 ? key.daysRemaining / key.lifetimeDays : 0;
      const percent = Math.round(Math.min(Math.max(ratio, 0), 1) * 100);
      const status = statusOf(key, percent);
      return { ...key, percent, status, tone: TONE[status] };
    });
}

export const ROTATION_STEP_NAMES: readonly StepName[] = ["GENERATE", "SEND_0800_161", "PARTNER_CONFIRM", "ACTIVATE"];

export function rotationSteps(rotation: KeyRotation | undefined): { name: StepName; status: StepStatus }[] {
  return ROTATION_STEP_NAMES.map((name) => ({
    name,
    status: rotation?.steps.find((s) => s.name === name)?.status ?? "PENDING",
  }));
}

// A made-up card (not in contracts/fixtures/cards.json, not Luhn-valid): the visualiser never sees a real PAN.
export const ILLUSTRATION_PAN = "9704361234567890";
export const ILLUSTRATION_PAN_MASKED = "9704 36•• •••• 7890";

const PIN_PATTERN = /^[0-9]{4,12}$/;
export const isValidPin = (pin: string) => PIN_PATTERN.test(pin);

const nibbles = (hex: string) => hex.replace(/(.{4})/g, "$1 ").trim();

// ponytail: an FNV-style hash standing in for 3DES/AES, as in the canvas; the screen labels these rows illustrative.
function illustrativeCipher(clear: string, salt: string): string {
  let h1 = 0x811c9dc5;
  let h2 = 0x1234567;
  const text = salt + clear;
  for (let i = 0; i < text.length; i++) {
    h1 = Math.imul(h1 ^ text.charCodeAt(i), 16777619) >>> 0;
    h2 = Math.imul(h2 + text.charCodeAt(i), 2654435761) >>> 0;
  }
  return (h1.toString(16).padStart(8, "0") + h2.toString(16).padStart(8, "0")).toUpperCase();
}

export type PinBlockRowId = "pinField" | "panField" | "clear" | "underTpk" | "underZpk";

/** The five canvas rows for ISO 9564 format 0. The PIN digits themselves are masked in the first row. */
export function pinBlockRows(pin: string, zpkKcv: string): { id: PinBlockRowId; value: string }[] {
  const pinField = ("0" + pin.length.toString(16).toUpperCase() + pin).padEnd(16, "F");
  const maskedPinField = [...pinField].map((ch, i) => (i >= 2 && i < 2 + pin.length ? "•" : ch)).join("");
  const panField = "0000" + ILLUSTRATION_PAN.slice(0, -1).slice(-12);
  const clear = buildPinBlock(pin, ILLUSTRATION_PAN);
  return [
    { id: "pinField", value: nibbles(maskedPinField) },
    { id: "panField", value: nibbles(panField) },
    { id: "clear", value: nibbles(clear) },
    { id: "underTpk", value: nibbles(illustrativeCipher(clear, "TPK")) },
    { id: "underZpk", value: nibbles(illustrativeCipher(clear, "ZPK" + zpkKcv)) },
  ];
}
