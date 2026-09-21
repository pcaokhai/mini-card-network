import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { duration } from "@/shared/motion/tokens";
import { useMotionPreference } from "@/shared/motion/useMotionPreference";

function stubMatchMedia(matches: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
}

describe("useMotionPreference", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("uses a short fade without movement when reduced motion is requested __MCN_004_AC3", () => {
    stubMatchMedia(true);
    const { result } = renderHook(() => useMotionPreference());
    expect(result.current.reduced).toBe(true);
    expect(result.current.enter.initial).toEqual({ opacity: 0 });
    expect(result.current.enter.transition.duration).toBe(duration.reducedFade);
  });

  it("uses the base enter motion otherwise __MCN_004_AC3", () => {
    stubMatchMedia(false);
    const { result } = renderHook(() => useMotionPreference());
    expect(result.current.enter.initial).toEqual({ opacity: 0, y: 8 });
  });
});
