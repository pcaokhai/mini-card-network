import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { useMessageLab } from "@/shared/state/message-lab";
import { BitmapGrid } from "./BitmapGrid";

// jsdom has no matchMedia; useMotionPreference (via BitmapGrid) needs it stubbed,
// same as src/shared/motion/useMotionPreference.test.ts does.
function stubMatchMedia(matches: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
}

describe("BitmapGrid", () => {
  afterEach(() => vi.unstubAllGlobals());
  beforeEach(() => {
    stubMatchMedia(false);
    act(() =>
      useMessageLab.setState({
        decoded: { mti: "0800", primaryBitmap: "8220000000000000", secondaryBitmap: null, segments: [], fields: [] },
        selectedKey: null,
        raw: "",
        setRaw: () => {},
        setDecoded: () => {},
        select: useMessageLab.getState().select,
      }),
    );
  });

  it("renders 64 cells and selects DE 11 when its bit is clicked __MCN_104_AC2", async () => {
    renderWithIntl(<BitmapGrid page="primary" />);
    const cells = screen.getAllByRole("button", { name: /^bit /i });
    expect(cells).toHaveLength(64);
    // 0x8220000000000000 has bits 1, 3, 11 set (1-indexed) -> DE 1 (secondary marker), 3, 11.
    await userEvent.click(screen.getByRole("button", { name: "bit 11" }));
    expect(useMessageLab.getState().selectedKey).toBe("11");
  });

  it("does not render a secondary tab when bit 1 is unset", () => {
    renderWithIntl(<BitmapGrid page="primary" />);
    expect(screen.queryByRole("tab", { name: /secondary/i })).not.toBeInTheDocument();
  });
});
