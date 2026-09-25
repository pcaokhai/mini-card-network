import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { CardPicker } from "./CardPicker";

const server = setupServer(
  http.get("*/v1/cards/:cardRef", ({ params }) =>
    params.cardRef === "crd_normal0001"
      ? HttpResponse.json({ availableBalance: { amount: 5_000_000, currency: "704" } })
      : new HttpResponse(null, { status: 404 }),
  ),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterAll(() => server.close());

describe("CardPicker", () => {
  it("MCN-305-AC1 renders the six fixture cards with tag, last four and balance, never a PAN", async () => {
    renderWithIntl(<CardPicker selected="tok_normal" onSelect={vi.fn()} />);
    const tiles = screen.getAllByRole("button");
    expect(tiles).toHaveLength(6);
    expect(tiles[0]).toHaveTextContent("Bình thường");
    expect(tiles[0]).toHaveTextContent("•••• 4417");
    expect(await screen.findByText("Số dư 5.000.000 ₫")).toBeInTheDocument();
    expect(tiles[5]).toHaveTextContent("Số dư lớn");
    for (const tile of tiles) expect(tile.textContent).not.toMatch(/\d{12,}/);
  });

  it("leaves the balance line out when the card API has no balance", async () => {
    renderWithIntl(<CardPicker selected={null} onSelect={vi.fn()} />);
    await screen.findByText("Số dư 5.000.000 ₫");
    expect(screen.getAllByText(/^Số dư/)).toHaveLength(1);
  });

  it("marks the selected card and reports a newly pressed one", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    renderWithIntl(<CardPicker selected="tok_normal" onSelect={onSelect} />);
    expect(screen.getAllByRole("button")[0]).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: /3310/ }));
    expect(onSelect).toHaveBeenCalledWith("tok_blocked");
  });
});
