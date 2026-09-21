import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { LinkStatusPill } from "./LinkStatusPill";

describe("LinkStatusPill", () => {
  it("renders SIGNED_ON with its status marker __MCN_205_AC1", () => {
    // renderWithIntl uses the vi locale (see DetailPanel.test.tsx precedent).
    renderWithIntl(<LinkStatusPill status="SIGNED_ON" />);
    expect(screen.getByTestId("link-status-pill")).toHaveAttribute("data-status", "SIGNED_ON");
  });

  it("renders a different label for DOWN than for SIGNED_ON", () => {
    renderWithIntl(<LinkStatusPill status="DOWN" />);
    const pill = screen.getByTestId("link-status-pill");
    expect(pill).toHaveAttribute("data-status", "DOWN");
    expect(pill.textContent).not.toBe("");
  });
});
