import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Keypad } from "./Keypad";

describe("Keypad", () => {
  it("appends a digit on click up to maxLength", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Keypad value="12" onChange={onChange} maxLength={4} />);
    await user.click(screen.getByRole("button", { name: "3" }));
    expect(onChange).toHaveBeenCalledWith("123");
  });

  it("responds to physical keyboard digit presses", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Keypad value="" onChange={onChange} maxLength={4} />);
    await user.keyboard("5");
    expect(onChange).toHaveBeenCalledWith("5");
  });

  it("does not exceed maxLength", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Keypad value="1234" onChange={onChange} maxLength={4} />);
    await user.click(screen.getByRole("button", { name: "5" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("removes the last digit on backspace click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Keypad value="123" onChange={onChange} maxLength={4} />);
    await user.click(screen.getByRole("button", { name: /backspace/i }));
    expect(onChange).toHaveBeenCalledWith("12");
  });
});
