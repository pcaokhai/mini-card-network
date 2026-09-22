import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { PlaybackControls } from "./PlaybackControls";

describe("PlaybackControls", () => {
  it("calls onStepChange(currentStep + 1) on next", async () => {
    const user = userEvent.setup();
    const onStepChange = vi.fn();
    renderWithIntl(
      <PlaybackControls
        currentStep={1}
        totalSteps={4}
        isPlaying={false}
        onStepChange={onStepChange}
        onTogglePlay={vi.fn()}
        onRestart={vi.fn()}
      />,
    );
    await user.click(screen.getByTestId("journey-next"));
    expect(onStepChange).toHaveBeenCalledWith(2);
  });

  it("disables next at the last step", () => {
    renderWithIntl(
      <PlaybackControls
        currentStep={3}
        totalSteps={4}
        isPlaying={false}
        onStepChange={vi.fn()}
        onTogglePlay={vi.fn()}
        onRestart={vi.fn()}
      />,
    );
    expect(screen.getByTestId("journey-next")).toBeDisabled();
  });

  it("disables back at the first step", () => {
    renderWithIntl(
      <PlaybackControls
        currentStep={0}
        totalSteps={4}
        isPlaying={false}
        onStepChange={vi.fn()}
        onTogglePlay={vi.fn()}
        onRestart={vi.fn()}
      />,
    );
    expect(screen.getByTestId("journey-back")).toBeDisabled();
  });

  it("calls onRestart", async () => {
    const user = userEvent.setup();
    const onRestart = vi.fn();
    renderWithIntl(
      <PlaybackControls
        currentStep={3}
        totalSteps={4}
        isPlaying={false}
        onStepChange={vi.fn()}
        onTogglePlay={vi.fn()}
        onRestart={onRestart}
      />,
    );
    await user.click(screen.getByTestId("journey-restart"));
    expect(onRestart).toHaveBeenCalled();
  });

  it("calls onTogglePlay and shows pause label while playing", async () => {
    const user = userEvent.setup();
    const onTogglePlay = vi.fn();
    renderWithIntl(
      <PlaybackControls
        currentStep={0}
        totalSteps={4}
        isPlaying={true}
        onStepChange={vi.fn()}
        onTogglePlay={onTogglePlay}
        onRestart={vi.fn()}
      />,
    );
    await user.click(screen.getByTestId("journey-toggle-play"));
    expect(onTogglePlay).toHaveBeenCalled();
  });
});
