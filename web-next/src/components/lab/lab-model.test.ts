import { describe, expect, it } from "vitest";
import type { DecodedMessage } from "@/shared/api/lab-client";
import { type Copy, bitmapRows, describeSelection, fieldRows, isBitOn, rawSegments } from "./lab-model";

// Copy stub: echoes the key and its values so assertions read which message was chosen.
const copy: Copy = {
  t: (key, values) => (values ? `${key}${JSON.stringify(values)}` : key),
  has: (key) => !key.includes("classes.9"),
};

const echo: DecodedMessage = {
  mti: "0800",
  primaryBitmap: "8220000000000000",
  secondaryBitmap: "0400000000000000",
  segments: [
    { key: "mti", text: "0800" },
    { key: "primaryBitmap", text: "8220000000000000" },
    { key: "secondaryBitmap", text: "0400000000000000" },
    { key: "7", text: "0921073300" },
    { key: "11", text: "000200" },
    { key: "70", text: "301" },
  ],
  fields: [
    { de: "7", easyName: "", technicalName: "", format: "n", value: "0921073300" },
    { de: "11", easyName: "", technicalName: "", format: "n", value: "000200" },
    { de: "70", easyName: "", technicalName: "", format: "n", value: "301" },
  ],
};

const withField = (de: string, value: string): DecodedMessage => ({
  ...echo,
  mti: "0200",
  fields: [{ de, easyName: "Backend name", technicalName: "Backend tech", format: "n", value }],
});

describe("lab-model", () => {
  it("merges both bitmaps into one segment, as on the wire __MCN_104_AC1", () => {
    expect(rawSegments(echo).map((s) => [s.key, s.text])).toEqual([
      ["mti", "0800"],
      ["bmp", "82200000000000000400000000000000"],
      ["7", "0921073300"],
      ["11", "000200"],
      ["70", "301"],
    ]);
  });

  it("reads bits from both bitmaps; bit 1 is on when a secondary bitmap exists __MCN_104_AC2", () => {
    expect([1, 7, 11, 70, 2, 65].map((n) => isBitOn(echo, n))).toEqual([true, true, true, true, false, false]);
  });

  it("gives each bitmap row its 8 bit numbers, hex and binary __MCN_104_AC2", () => {
    const rows = bitmapRows(echo, 1);
    expect(rows).toHaveLength(8);
    expect(rows[0]).toEqual({ bits: [65, 66, 67, 68, 69, 70, 71, 72], hex: "04", bin: "00000100" });
    expect(bitmapRows(echo, 0)[1]).toMatchObject({ bits: [9, 10, 11, 12, 13, 14, 15, 16], hex: "20", bin: "00100000" });
  });

  it("breaks the MTI into version, class, function and origin __MCN_104_AC3", () => {
    const detail = describeSelection(echo, "mti", copy);
    expect(detail).toMatchObject({ tag: "MTI", value: "0800", on: true, fmt: "n 4" });
    expect(detail.parts).toEqual([
      { k: "0 mti.version", v: "mti.versions.0" },
      { k: "8 mti.class", v: "mti.classes.8" },
      { k: "0 mti.function", v: "mti.functions.0" },
      { k: "0 mti.origin", v: "mti.origins.0" },
    ]);
  });

  it("falls back to a dash for an MTI digit the glossary does not know", () => {
    expect(describeSelection({ ...echo, mti: "0900" }, "mti", copy).parts[1]).toEqual({ k: "9 mti.class", v: "—" });
  });

  it("describes the whole bitmap with the field count", () => {
    expect(describeSelection(echo, "bmp", copy)).toMatchObject({
      tag: "Bitmap",
      title: "detail.bitmapTitleBoth",
      fmt: "b 128",
      value: "82200000000000000400000000000000",
      status: 'detail.fieldsPresent{"count":3}',
    });
  });

  it("splits DE 22 into card-read method and PIN capability __MCN_104_AC3", () => {
    expect(describeSelection(withField("22", "051"), "22", copy).parts).toEqual([
      { k: "05", v: "entry.pan.05" },
      { k: "1", v: "entry.pin.1" },
    ]);
  });

  it("lists the EMV tags found in DE 55 __MCN_104_AC3", () => {
    const parts = describeSelection(withField("55", "9F2608A1B2C3D4E5F607189F2701809F3602001C"), "55", copy).parts;
    expect(parts).toEqual([
      { k: "9F26", v: "emv.9F26" },
      { k: "9F27", v: "emv.9F27" },
      { k: "9F36", v: "emv.9F36" },
    ]);
  });

  it("splits DE 90 into the original message's parts __MCN_104_AC3", () => {
    const parts = describeSelection(withField("90", "020000012409210732440000097049900000000000"), "90", copy).parts;
    expect(parts).toEqual([
      { k: "0200", v: "original.mti" },
      { k: "000124", v: "original.stan" },
      { k: "0921073244", v: "original.time" },
      { k: "00000970499", v: "original.acquirer" },
    ]);
  });

  it("explains a known field that is not in the message", () => {
    expect(describeSelection(echo, "39", copy)).toMatchObject({
      tag: 'detail.fieldTag{"n":39}',
      title: "fields.39.name",
      tech: "Response code",
      fmt: "an 2",
      value: "—",
      on: false,
      status: "detail.absent",
      why: 'detail.absentWhy{"mti":"0800","why":"fields.39.why"}',
    });
  });

  it("shows the secondary bitmap as bit 1's value", () => {
    expect(describeSelection(echo, "1", copy)).toMatchObject({ on: true, value: "0400000000000000", status: "detail.present" });
  });

  it("uses the backend's names for a present field the glossary lacks", () => {
    expect(describeSelection(withField("60", "ABC"), "60", copy)).toMatchObject({
      title: "Backend name",
      tech: "Backend tech",
      fmt: "n",
      value: "ABC",
      why: "detail.unlistedWhy",
    });
  });

  it("names rows in plain words in Easy mode and technically in Expert mode __MCN_104_AC4", () => {
    expect(fieldRows(echo, false, copy)[1]).toEqual({ n: "11", name: "fields.11.name", fmt: "n 6", value: "000200" });
    expect(fieldRows(echo, true, copy)[1]?.name).toBe("System trace audit number (STAN)");
  });
});
