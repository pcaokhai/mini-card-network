import { http, HttpResponse, type HttpHandler } from "msw";
import type { KeyInfo, KeyRotation } from "@/shared/api/security-client";

// The six keys of Security.dc.html, with the canvas's KCVs and lifetimes. Only KCVs: no key material.
const CANVAS_KEYS: readonly KeyInfo[] = [
  { keyType: "ZMK", counterparty: "issuer", kcv: "8C21D4", status: "ACTIVE", daysRemaining: 312, lifetimeDays: 365 },
  { keyType: "ZPK", counterparty: "issuer", kcv: "3F9A21", status: "ACTIVE", daysRemaining: 26, lifetimeDays: 30 },
  { keyType: "ZAK", counterparty: "issuer", kcv: "51E0A7", status: "ACTIVE", daysRemaining: 5, lifetimeDays: 30 },
  { keyType: "TPK", counterparty: null, kcv: "A4F3C9", status: "ACTIVE", daysRemaining: 74, lifetimeDays: 90 },
  { keyType: "CVK", counterparty: null, kcv: "0B77E2", status: "ACTIVE", daysRemaining: 200, lifetimeDays: 365 },
  { keyType: "PVK", counterparty: null, kcv: "D19E40", status: "ACTIVE", daysRemaining: 200, lifetimeDays: 365 },
];
const NEW_ZPK_KCV = "7D02B1";
/** One step completes per interval, so the stepper visibly walks through the four steps. */
export const MOCK_ROTATION_STEP_MS = 1_200;
const STEP_NAMES = ["GENERATE", "SEND_0800_161", "PARTNER_CONFIRM", "ACTIVATE"] as const;

let rotation: { id: string; keyType: string; startedAt: number } | null = null;
let rotated = false;

export function resetSecurityMock() {
  rotation = null;
  rotated = false;
}

export function mockSecurityKeys(): KeyInfo[] {
  currentRotation();
  return CANVAS_KEYS.map((k) => (k.keyType === "ZPK" && rotated ? { ...k, kcv: NEW_ZPK_KCV, daysRemaining: 30 } : k));
}

function currentRotation(): KeyRotation | null {
  if (rotation === null) return null;
  const done = Math.min(STEP_NAMES.length, Math.floor((Date.now() - rotation.startedAt) / MOCK_ROTATION_STEP_MS) + 1);
  const completed = done === STEP_NAMES.length;
  if (completed && rotation.keyType === "ZPK") rotated = true;
  return {
    rotationId: rotation.id,
    keyType: rotation.keyType,
    status: completed ? "COMPLETED" : "RUNNING",
    newKcv: completed ? NEW_ZPK_KCV : null,
    steps: STEP_NAMES.map((name, i) => ({ name, status: i < done ? "DONE" : "PENDING", completedAt: null })),
  };
}

/** Security and Keys under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const securityHandlers: HttpHandler[] = [
  http.get("*/v1/keys/acquirer", () => HttpResponse.json(mockSecurityKeys())),
  http.post("*/v1/keys/acquirer/rotations", async ({ request }) => {
    // The id comes from the Idempotency-Key, so a replayed request (MockProvider's worker can see one
    // twice) lands on the same rotation instead of replacing it.
    const id = `rot_${request.headers.get("Idempotency-Key") ?? Date.now()}`;
    const { keyType } = (await request.json()) as { keyType: string };
    if (rotation?.id !== id) rotation = { id, keyType, startedAt: Date.now() };
    return HttpResponse.json(currentRotation(), { status: 202 });
  }),
  http.get("*/v1/keys/acquirer/rotations/:rotationId", ({ params }) => {
    const current = currentRotation();
    return current?.rotationId === params.rotationId ? HttpResponse.json(current) : new HttpResponse(null, { status: 404 });
  }),
];
