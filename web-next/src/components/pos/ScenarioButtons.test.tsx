import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ScenarioButtons } from "./ScenarioButtons";

describe("ScenarioButtons", () => {
  it("renders all 5 one-click scenarios", () => {
    render(<ScenarioButtons onPick={vi.fn()} />);
    expect(screen.getAllByRole("button")).toHaveLength(5);
  });

  it("calls onPick with the scenario id", async () => {
    const user = userEvent.setup();
    const onPick = vi.fn();
    render(<ScenarioButtons onPick={onPick} />);
    await user.click(screen.getByRole("button", { name: /approved/i }));
    expect(onPick).toHaveBeenCalledWith("approved");
  });
});
