import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { CountdownRing } from "./CountdownRing";

describe("CountdownRing", () => {
  it("renders an svg ring marked as playing when isPlaying is true (MCN-406-AC2)", () => {
    renderWithIntl(<CountdownRing durationMs={2000} isPlaying={true} />);
    expect(screen.getByTestId("countdown-ring")).toHaveAttribute("data-playing", "true");
  });

  it("pauses the ring when isPlaying is false (MCN-406-AC2)", () => {
    renderWithIntl(<CountdownRing durationMs={2000} isPlaying={false} />);
    expect(screen.getByTestId("countdown-ring")).toHaveAttribute("data-playing", "false");
  });
});
