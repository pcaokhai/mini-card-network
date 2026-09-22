import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ScenarioCard } from "@/components/chaos/ScenarioCard";

describe("ScenarioCard", () => {
  it("calls onToggle with the scenario id and inverted enabled state __MCN_405_AC1", () => {
    const onToggle = vi.fn();
    render(
      <ScenarioCard
        scenario={{ id: "SLOW_NETWORK", enabled: false, easyText: "Slow", technicalText: "3000ms latency" }}
        onToggle={onToggle}
      />,
    );

    fireEvent.click(screen.getByRole("switch"));

    expect(onToggle).toHaveBeenCalledWith("SLOW_NETWORK", true);
  });

  it("renders active state visually distinct via data-enabled __MCN_405_AC1", () => {
    render(
      <ScenarioCard
        scenario={{ id: "ISSUER_DOWN", enabled: true, easyText: "Down", technicalText: "reset_peer" }}
        onToggle={vi.fn()}
      />,
    );
    expect(screen.getByTestId("scenario-card-ISSUER_DOWN")).toHaveAttribute("data-enabled", "true");
  });
});
