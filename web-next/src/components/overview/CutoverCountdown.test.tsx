import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { CutoverCountdown } from "./CutoverCountdown";

describe("CutoverCountdown", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 23, 59, 0));
  });
  afterEach(() => vi.useRealTimers());

  it("counts down to 23:59:59 local and ticks every second", () => {
    renderWithIntl(<CutoverCountdown />);
    expect(screen.getByTestId("cutover-remaining")).toHaveTextContent("00:00:59");
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByTestId("cutover-remaining")).toHaveTextContent("00:00:58");
  });
});
