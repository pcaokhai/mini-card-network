import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { useMessageLab } from "@/shared/state/message-lab";
import { RawSegments } from "./RawSegments";

const decoded = {
  mti: "0800",
  primaryBitmap: "8220000000000000",
  secondaryBitmap: null,
  segments: [
    { key: "mti", text: "0800" },
    { key: "primaryBitmap", text: "8220000000000000" },
    { key: "7", text: "0921073300" },
    { key: "11", text: "000200" },
    { key: "70", text: "301" },
  ],
  fields: [],
};

describe("RawSegments", () => {
  beforeEach(() => act(() => useMessageLab.setState({ decoded, selectedKey: null, raw: "", setRaw: () => {}, setDecoded: () => {}, select: useMessageLab.getState().select })));

  it("renders one coloured span per segment and selects it on click __MCN_104_AC1", async () => {
    renderWithIntl(<RawSegments />);
    const stan = screen.getByText("000200");
    await userEvent.click(stan);
    expect(useMessageLab.getState().selectedKey).toBe("11");
  });

  it("marks the currently selected segment", () => {
    act(() => useMessageLab.getState().select("70"));
    renderWithIntl(<RawSegments />);
    expect(screen.getByText("301")).toHaveAttribute("aria-current", "true");
  });
});
