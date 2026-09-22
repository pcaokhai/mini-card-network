import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PinPad } from "./PinPad";

describe("PinPad", () => {
  it("masks entered digits and clears state after submit", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<PinPad onSubmit={onSubmit} />);
    for (const digit of ["1", "2", "3", "4"]) {
      await user.click(screen.getByRole("button", { name: digit }));
    }
    expect(screen.getByLabelText(/pin/i)).toHaveTextContent("••••");
    await user.click(screen.getByRole("button", { name: /confirm/i }));
    expect(onSubmit).toHaveBeenCalledWith("1234");
    expect(screen.getByLabelText(/pin/i)).toHaveTextContent("");
  });

  it("disables confirm until 4 digits are entered", async () => {
    const user = userEvent.setup();
    render(<PinPad onSubmit={vi.fn()} />);
    expect(screen.getByRole("button", { name: /confirm/i })).toBeDisabled();
    for (const digit of ["1", "2", "3"]) {
      await user.click(screen.getByRole("button", { name: digit }));
    }
    expect(screen.getByRole("button", { name: /confirm/i })).toBeDisabled();
  });
});
