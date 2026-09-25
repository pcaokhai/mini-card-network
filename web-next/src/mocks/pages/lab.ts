import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";

type DecodedMessage = components["schemas"]["DecodedMessage"];

interface CanvasMessage {
  mti: string;
  label: string;
  fields: Record<number, string>;
}

// The four sample messages of the design canvas (MessageLab.dc.html `msgs`), PAN already masked.
const PAN = "970436******4417";
const CANVAS_MESSAGES: CanvasMessage[] = [
  {
    mti: "0200",
    label: "Purchase, chip + PIN",
    fields: {
      2: PAN, 3: "000000", 4: "000000250000", 7: "0921073208", 11: "000123", 12: "143208", 13: "0921", 14: "2811",
      18: "5814", 22: "051", 25: "00", 32: "970499", 37: "626514000123", 41: "00000042", 42: "GOCPHO000000001",
      49: "704", 52: "7A3F09C21B84D6E0", 55: "9F2608A1B2C3D4E5F607189F2701809F3602001C", 64: "4E1D7B02C9A3F815",
    },
  },
  {
    mti: "0210",
    label: "Purchase approved",
    fields: {
      2: PAN, 3: "000000", 4: "000000250000", 7: "0921073208", 11: "000123", 12: "143208", 13: "0921", 32: "970499",
      37: "626514000123", 38: "A00123", 39: "00", 41: "00000042", 42: "GOCPHO000000001", 49: "704", 64: "91C0A4E27D3B5F68",
    },
  },
  {
    mti: "0420",
    label: "Reversal on timeout",
    fields: {
      2: PAN, 3: "000000", 4: "000000600000", 7: "0921073314", 11: "000125", 12: "143244", 13: "0921", 32: "970499",
      37: "626514000124", 39: "68", 41: "00000042", 42: "GOCPHO000000001", 49: "704",
      90: "020000012409210732440000097049900000000000",
    },
  },
  { mti: "0800", label: "Echo (network management)", fields: { 7: "0921073300", 11: "000200", 70: "301" } },
];

// DE 2/32 are LLVAR (DE 2's length counts the clear PAN's 16 digits), DE 55 is LLLVAR in bytes.
function wireText(de: number, value: string): string {
  if (de === 2 || de === 32) return String(de === 2 ? 16 : value.length).padStart(2, "0") + value;
  if (de === 55) return String(value.length / 2).padStart(3, "0") + value;
  return value;
}

function bitmapHex(present: number[], page: 0 | 1): string {
  const isOn = (de: number) => (de === 1 ? present.some((n) => n > 64) : present.includes(de));
  return Array.from({ length: 8 }, (_, b) => {
    const byte = Array.from({ length: 8 }, (_, j) => (isOn(page * 64 + b * 8 + j + 1) ? "1" : "0")).join("");
    return parseInt(byte, 2).toString(16).toUpperCase().padStart(2, "0");
  }).join("");
}

function decode(message: CanvasMessage): DecodedMessage {
  const present = Object.keys(message.fields).map(Number).sort((a, b) => a - b);
  const primaryBitmap = bitmapHex(present, 0);
  const secondaryBitmap = present.some((n) => n > 64) ? bitmapHex(present, 1) : null;
  const valueOf = (de: number) => message.fields[de] ?? "";
  return {
    mti: message.mti,
    primaryBitmap,
    secondaryBitmap,
    segments: [
      { key: "mti", text: message.mti },
      { key: "primaryBitmap", text: primaryBitmap },
      ...(secondaryBitmap ? [{ key: "secondaryBitmap", text: secondaryBitmap }] : []),
      ...present.map((de) => ({ key: String(de), text: wireText(de, valueOf(de)) })),
    ],
    // The screen names fields from its own glossary (messages/*.json lab.fields), so these stay terse.
    fields: present.map((de) => ({ de: String(de), easyName: `DE ${de}`, technicalName: `DE ${de}`, format: "", value: valueOf(de) })),
  };
}

/** Also seeds the Message Lab stories, so Storybook shows exactly what `pnpm dev:mock` serves. */
export const LAB_SAMPLES = CANVAS_MESSAGES.map((m) => {
  const decoded = decode(m);
  return { mti: m.mti, label: m.label, raw: decoded.segments.map((s) => s.text).join(""), decoded };
});

/** Message Lab under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const labHandlers = [
  http.get("*/v1/lab/messages/samples", () => HttpResponse.json(LAB_SAMPLES.map(({ mti, label, raw }) => ({ mti, label, raw })))),
  http.post("*/v1/lab/messages/decode", async ({ request }) => {
    const { raw } = (await request.json()) as { raw: string };
    const sample = LAB_SAMPLES.find((s) => s.raw === raw);
    return sample
      ? HttpResponse.json(sample.decoded)
      : HttpResponse.json({ type: "about:blank", title: "Not a lab sample", status: 422 }, { status: 422 });
  }),
];
