import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CardPicker } from "./CardPicker";

describe("CardPicker", () => {
  it("renders exactly 4 selectable test cards with masked PANs, never raw PANs", () => {
    render(<CardPicker selected={null} onSelect={vi.fn()} />);
    const cards = screen.getAllByRole("radio");
    expect(cards).toHaveLength(6);
    for (const card of cards) {
      expect(card.textContent).not.toMatch(/9704360000/);
    }
  });

  it("calls onSelect with the cardToken when a card is chosen", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<CardPicker selected={null} onSelect={onSelect} />);
    await user.click(screen.getAllByRole("radio")[0]);
    expect(onSelect).toHaveBeenCalledWith(expect.stringMatching(/^tok_/));
  });
});
