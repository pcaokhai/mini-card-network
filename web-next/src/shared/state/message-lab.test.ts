import { act } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useMessageLab } from "@/shared/state/message-lab";

describe("useMessageLab", () => {
  beforeEach(() => act(() => useMessageLab.setState({ raw: "", decoded: null, selectedKey: null })));

  it("tracks the selected element id, shared by every view __MCN_104_AC1", () => {
    act(() => useMessageLab.getState().select("11"));
    expect(useMessageLab.getState().selectedKey).toBe("11");
    act(() => useMessageLab.getState().select(null));
    expect(useMessageLab.getState().selectedKey).toBeNull();
  });

  it("clears the selection when a new message is decoded", () => {
    act(() => {
      useMessageLab.getState().select("11");
      useMessageLab.getState().setDecoded({ mti: "0800", primaryBitmap: "0".repeat(16), secondaryBitmap: null, segments: [], fields: [] });
    });
    expect(useMessageLab.getState().selectedKey).toBeNull();
  });
});
