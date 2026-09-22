import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { ReactionPanel } from "@/components/chaos/ReactionPanel";

describe("ReactionPanel", () => {
  it("lists one reaction per active scenario, hides technical text outside expert mode __MCN_405_AC3", () => {
    renderWithIntl(<ReactionPanel activeScenarios={["SLOW_NETWORK", "ISSUER_DOWN"]} expertMode={false} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(document.querySelector("[data-expert-only]")).not.toBeInTheDocument();
  });

  it("also shows technical text in expert mode __MCN_405_AC3", () => {
    renderWithIntl(<ReactionPanel activeScenarios={["SLOW_NETWORK"]} expertMode={true} />);
    expect(document.querySelector("[data-expert-only]")).toBeInTheDocument();
  });

  it("shows an empty state with no active scenarios __MCN_405_AC3", () => {
    renderWithIntl(<ReactionPanel activeScenarios={[]} expertMode={false} />);
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });
});
