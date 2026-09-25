import type { DecodedMessage } from "@/shared/api/lab-client";

/** Translator for the `lab` namespace; `has` lets glossary lookups fall back instead of throwing. */
export interface Copy {
  t: (key: string, values?: Record<string, string | number>) => string;
  has: (key: string) => boolean;
}

export interface Segment {
  key: string;
  text: string;
  kind: "mti" | "bmp" | "field";
}

export interface Detail {
  tag: string;
  title: string;
  why: string;
  tech: string;
  fmt: string;
  value: string;
  status: string;
  on: boolean;
  parts: { k: string; v: string }[];
}

export interface FieldRow {
  n: string;
  name: string;
  fmt: string;
  value: string;
}

// Technical name and format of every DE the lab explains (canvas `D`, plus 15, 43 and 128 that the
// real gateway's samples carry). Plain-language names and explanations live in messages/*.json.
const FIELD_SPECS: Record<number, { tech: string; fmt: string }> = {
  1: { tech: "Secondary bitmap", fmt: "b 64" },
  2: { tech: "Primary account number (PAN)", fmt: "n..19 LLVAR" },
  3: { tech: "Processing code", fmt: "n 6" },
  4: { tech: "Amount, transaction", fmt: "n 12" },
  7: { tech: "Transmission date and time", fmt: "n 10 MMDDhhmmss" },
  11: { tech: "System trace audit number (STAN)", fmt: "n 6" },
  12: { tech: "Time, local transaction", fmt: "n 6 hhmmss" },
  13: { tech: "Date, local transaction", fmt: "n 4 MMDD" },
  14: { tech: "Date, expiration", fmt: "n 4 YYMM" },
  15: { tech: "Date, settlement", fmt: "n 4 MMDD" },
  18: { tech: "Merchant category code (MCC)", fmt: "n 4" },
  22: { tech: "POS entry mode", fmt: "n 3" },
  25: { tech: "POS condition code", fmt: "n 2" },
  32: { tech: "Acquiring institution ID", fmt: "n..11 LLVAR" },
  37: { tech: "Retrieval reference number (RRN)", fmt: "an 12" },
  38: { tech: "Authorization ID response", fmt: "an 6" },
  39: { tech: "Response code", fmt: "an 2" },
  41: { tech: "Card acceptor terminal ID", fmt: "ans 8" },
  42: { tech: "Card acceptor ID code", fmt: "ans 15" },
  43: { tech: "Card acceptor name/location", fmt: "ans 40" },
  49: { tech: "Currency code, transaction", fmt: "n 3" },
  52: { tech: "PIN data (PIN block)", fmt: "b 64" },
  55: { tech: "ICC data (EMV)", fmt: "b..255 LLLVAR" },
  64: { tech: "Message authentication code (MAC)", fmt: "b 64" },
  70: { tech: "Network management information code", fmt: "n 3" },
  90: { tech: "Original data elements", fmt: "n 42" },
  128: { tech: "Message authentication code (MAC)", fmt: "b 64" },
};

const DASH = "—";

export function rawSegments(decoded: DecodedMessage): Segment[] {
  const bitmap = decoded.primaryBitmap + (decoded.secondaryBitmap ?? "");
  return [
    { key: "mti", text: decoded.mti, kind: "mti" },
    { key: "bmp", text: bitmap, kind: "bmp" },
    ...decoded.segments
      .filter((s) => s.key !== "mti" && s.key !== "primaryBitmap" && s.key !== "secondaryBitmap")
      .map((s) => ({ key: s.key, text: s.text, kind: "field" as const })),
  ];
}

function byteOf(decoded: DecodedMessage, page: 0 | 1, b: number): number {
  const hex = page === 0 ? decoded.primaryBitmap : decoded.secondaryBitmap;
  return hex ? parseInt(hex.slice(b * 2, b * 2 + 2), 16) : 0;
}

export function isBitOn(decoded: DecodedMessage, n: number): boolean {
  const page = n > 64 ? 1 : 0;
  const i = n - 1 - page * 64;
  return (byteOf(decoded, page, Math.floor(i / 8)) & (0x80 >> i % 8)) !== 0;
}

export function bitmapRows(decoded: DecodedMessage, page: 0 | 1) {
  return Array.from({ length: 8 }, (_, b) => {
    const v = byteOf(decoded, page, b);
    return {
      bits: Array.from({ length: 8 }, (_, j) => page * 64 + b * 8 + j + 1),
      hex: v.toString(16).toUpperCase().padStart(2, "0"),
      bin: v.toString(2).padStart(8, "0"),
    };
  });
}

function glossary(copy: Copy, key: string): string {
  return copy.has(key) ? copy.t(key) : DASH;
}

function fieldName(n: number, fallback: string | undefined, expert: boolean, copy: Copy): string {
  const spec = FIELD_SPECS[n];
  if (!spec) return fallback ?? copy.t("detail.fieldTag", { n });
  return expert ? spec.tech : copy.t(`fields.${n}.name`);
}

export function fieldRows(decoded: DecodedMessage, expert: boolean, copy: Copy): FieldRow[] {
  return decoded.fields.map((f) => {
    const n = Number(f.de);
    return {
      n: f.de,
      name: fieldName(n, expert ? f.technicalName : f.easyName, expert, copy),
      fmt: FIELD_SPECS[n]?.fmt ?? f.format,
      value: f.value,
    };
  });
}

function mtiDetail(mti: string, copy: Copy): Detail {
  const [version = "", cls = "", fn = "", origin = ""] = mti.split("");
  return {
    tag: "MTI",
    title: copy.t("detail.mtiTitle", { mti }),
    why: copy.t("detail.mtiWhy"),
    tech: "Message type indicator",
    fmt: "n 4",
    value: mti,
    status: copy.t("detail.always"),
    on: true,
    parts: [
      { k: `${version} ${copy.t("mti.version")}`, v: glossary(copy, `mti.versions.${version}`) },
      { k: `${cls} ${copy.t("mti.class")}`, v: glossary(copy, `mti.classes.${cls}`) },
      { k: `${fn} ${copy.t("mti.function")}`, v: glossary(copy, `mti.functions.${fn}`) },
      { k: `${origin} ${copy.t("mti.origin")}`, v: glossary(copy, `mti.origins.${origin}`) },
    ],
  };
}

function bitmapDetail(decoded: DecodedMessage, copy: Copy): Detail {
  const both = decoded.secondaryBitmap != null;
  return {
    tag: "Bitmap",
    title: copy.t(both ? "detail.bitmapTitleBoth" : "detail.bitmapTitlePrimary"),
    why: copy.t("detail.bitmapWhy"),
    tech: both ? "Primary + secondary bitmap" : "Primary bitmap",
    fmt: both ? "b 128" : "b 64",
    value: decoded.primaryBitmap + (decoded.secondaryBitmap ?? ""),
    status: copy.t("detail.fieldsPresent", { count: decoded.fields.length }),
    on: true,
    parts: [],
  };
}

/** EMV TLV: 2-byte tag when the first byte's low 5 bits are all set, then a 1-byte length. */
function emvTags(hex: string): string[] {
  const tags: string[] = [];
  let i = 0;
  while (i + 4 <= hex.length) {
    const tagLength = (parseInt(hex.slice(i, i + 2), 16) & 0x1f) === 0x1f ? 4 : 2;
    const tag = hex.slice(i, i + tagLength);
    const length = parseInt(hex.slice(i + tagLength, i + tagLength + 2), 16);
    if (Number.isNaN(length)) break;
    tags.push(tag);
    i += tagLength + 2 + length * 2;
  }
  return tags;
}

function fieldParts(n: number, value: string, copy: Copy): Detail["parts"] {
  if (n === 22) {
    return [
      { k: value.slice(0, 2), v: glossary(copy, `entry.pan.${value.slice(0, 2)}`) },
      { k: value.slice(2, 3), v: glossary(copy, `entry.pin.${value.slice(2, 3)}`) },
    ];
  }
  if (n === 55) return emvTags(value).map((tag) => ({ k: tag, v: copy.has(`emv.${tag}`) ? copy.t(`emv.${tag}`) : "" }));
  if (n === 90) {
    return [
      { k: value.slice(0, 4), v: copy.t("original.mti") },
      { k: value.slice(4, 10), v: copy.t("original.stan") },
      { k: value.slice(10, 20), v: copy.t("original.time") },
      { k: value.slice(20, 31), v: copy.t("original.acquirer") },
    ];
  }
  return [];
}

function fieldDetail(decoded: DecodedMessage, n: number, copy: Copy): Detail {
  const field = decoded.fields.find((f) => Number(f.de) === n);
  const spec = FIELD_SPECS[n];
  const on = n === 1 ? decoded.secondaryBitmap != null : field !== undefined;
  const value = n === 1 ? (decoded.secondaryBitmap ?? DASH) : (field?.value ?? DASH);
  const why = spec ? copy.t(`fields.${n}.why`) : on ? copy.t("detail.unlistedWhy") : copy.t("detail.unusedWhy");
  return {
    tag: copy.t("detail.fieldTag", { n }),
    title: spec ? copy.t(`fields.${n}.name`) : (field?.easyName ?? copy.t("detail.fieldTag", { n })),
    why: on ? why : copy.t("detail.absentWhy", { mti: decoded.mti, why }),
    tech: spec?.tech ?? field?.technicalName ?? copy.t("detail.unusedTech"),
    fmt: spec?.fmt ?? (field?.format || DASH),
    value,
    status: copy.t(on ? "detail.present" : "detail.absent"),
    on,
    parts: on && field ? fieldParts(n, field.value, copy) : [],
  };
}

/** MCN-104-AC3: what the dark detail panel says about the selected MTI, bitmap or field. */
export function describeSelection(decoded: DecodedMessage, key: string, copy: Copy): Detail {
  if (key === "mti") return mtiDetail(decoded.mti, copy);
  if (key === "bmp") return bitmapDetail(decoded, copy);
  return fieldDetail(decoded, Number(key), copy);
}

/** The canvas opens each sample on its amount (DE 4), or DE 70 for a network management message. */
export function defaultSelection(decoded: DecodedMessage): string {
  return decoded.fields.some((f) => f.de === "4") ? "4" : decoded.fields.some((f) => f.de === "70") ? "70" : "mti";
}
