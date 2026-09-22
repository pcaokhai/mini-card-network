import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import type { components } from "@/shared/api/generated/schema";
import { StepTimeline } from "./StepTimeline";

type JourneyStep = components["schemas"]["JourneyStep"];

const steps: JourneyStep[] = [
  { seq: 0, actor: "POS", offsetMs: 0, title: "Purchase requested", easyText: "", technicalText: "", kind: "INFO", message: null },
  { seq: 1, actor: "ISSUER", offsetMs: 50, title: "Approved", easyText: "", technicalText: "", kind: "OK", message: null },
  { seq: 2, actor: "ACQUIRER", offsetMs: 60, title: "Response returned", easyText: "", technicalText: "", kind: "INFO", message: null },
];

describe("StepTimeline", () => {
  it("marks the current step and dims future steps", () => {
    renderWithIntl(<StepTimeline steps={steps} currentStep={1} onSelectStep={vi.fn()} />);
    const items = screen.getAllByRole("listitem");
    expect(items[1]).toHaveAttribute("data-state", "current");
    expect(items[0]).toHaveAttribute("data-state", "past");
    expect(items[2]).toHaveAttribute("data-state", "future");
  });

  it("calls onSelectStep when a step is clicked", async () => {
    const user = userEvent.setup();
    const onSelectStep = vi.fn();
    renderWithIntl(<StepTimeline steps={steps} currentStep={0} onSelectStep={onSelectStep} />);
    await user.click(screen.getAllByRole("listitem")[2]);
    expect(onSelectStep).toHaveBeenCalledWith(2);
  });
});
