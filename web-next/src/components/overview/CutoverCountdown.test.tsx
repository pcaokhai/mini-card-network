import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { CutoverCountdown } from "./CutoverCountdown";

describe("CutoverCountdown", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("counts down in hours and minutes to 23:59:59 local", () => {
    vi.setSystemTime(new Date(2026, 0, 1, 14, 32, 0));
    renderWithIntl(<CutoverCountdown />);
    expect(screen.getByTestId("cutover-remaining")).toHaveTextContent("9 giờ 27 phút");
  });

  it("ticks down as time passes", () => {
    vi.setSystemTime(new Date(2026, 0, 1, 23, 57, 0));
    renderWithIntl(<CutoverCountdown />);
    expect(screen.getByTestId("cutover-remaining")).toHaveTextContent("0 giờ 2 phút");
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByTestId("cutover-remaining")).toHaveTextContent("0 giờ 1 phút");
  });
});
