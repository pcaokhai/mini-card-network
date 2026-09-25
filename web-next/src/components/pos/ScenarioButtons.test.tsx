import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { ScenarioButtons } from "./ScenarioButtons";

describe("ScenarioButtons", () => {
  it("MCN-305-AC1 renders the five real-issuer scenarios as pills", () => {
    renderWithIntl(<ScenarioButtons selected="normal" onPick={vi.fn()} />);
    const labels = screen.getAllByRole("button").map((b) => b.textContent);
    expect(labels).toEqual(["Mua hàng bình thường", "Không đủ tiền", "Thẻ bị khóa", "Thẻ hết hạn", "Vượt hạn mức"]);
    expect(screen.getByRole("button", { name: "Mua hàng bình thường" })).toHaveAttribute("aria-pressed", "true");
  });

  it("reports the picked scenario", async () => {
    const onPick = vi.fn();
    renderWithIntl(<ScenarioButtons selected={null} onPick={onPick} />);
    await userEvent.click(screen.getByRole("button", { name: "Không đủ tiền" }));
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: "low", cardToken: "tok_low", amount: 350_000 }));
  });
});
