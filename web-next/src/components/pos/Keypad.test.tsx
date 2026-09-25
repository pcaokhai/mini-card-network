import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { Keypad } from "./Keypad";

describe("Keypad", () => {
  it("MCN-305-AC1 lays out 1-9, C, 0, ⌫ and reports each key", async () => {
    const user = userEvent.setup();
    const onKey = vi.fn();
    renderWithIntl(<Keypad onKey={onKey} />);
    const labels = screen.getAllByRole("button").map((b) => b.textContent);
    expect(labels).toEqual(["1", "2", "3", "4", "5", "6", "7", "8", "9", "C", "0", "⌫"]);
    await user.click(screen.getByRole("button", { name: "Xóa hết" }));
    await user.click(screen.getByRole("button", { name: "Xóa một số" }));
    await user.click(screen.getByRole("button", { name: "7" }));
    expect(onKey.mock.calls.map(([k]) => k)).toEqual(["C", "⌫", "7"]);
  });

  it("MCN-305-AC1 accepts digits and Backspace from the physical keyboard", async () => {
    const user = userEvent.setup();
    const onKey = vi.fn();
    renderWithIntl(<Keypad onKey={onKey} />);
    await user.keyboard("5{Backspace}");
    expect(onKey.mock.calls.map(([k]) => k)).toEqual(["5", "⌫"]);
  });

  it("ignores keys typed into a text input", async () => {
    const user = userEvent.setup();
    const onKey = vi.fn();
    renderWithIntl(
      <>
        <input aria-label="rrn" />
        <Keypad onKey={onKey} />
      </>,
    );
    await user.type(screen.getByLabelText("rrn"), "12");
    expect(onKey).not.toHaveBeenCalled();
  });

  it("ignores everything while disabled", async () => {
    const user = userEvent.setup();
    const onKey = vi.fn();
    renderWithIntl(<Keypad onKey={onKey} disabled />);
    await user.keyboard("5");
    expect(screen.getByRole("button", { name: "5" })).toBeDisabled();
    expect(onKey).not.toHaveBeenCalled();
  });
});
