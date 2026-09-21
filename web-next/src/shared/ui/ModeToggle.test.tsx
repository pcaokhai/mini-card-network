import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { useDisplayMode } from "@/shared/state/display-mode";
import { ModeToggle } from "@/shared/ui/ModeToggle";
import { Term } from "@/shared/ui/Term";
import { renderWithIntl } from "@/test/render";

describe("Display mode", () => {
  beforeEach(() => act(() => useDisplayMode.setState({ mode: "easy" })));

  it("switches every Term between easy and technical labels __MCN_004_AC2", async () => {
    renderWithIntl(
      <>
        <ModeToggle />
        <Term id="rrn" />
      </>,
    );
    expect(screen.getByText("Mã tra soát")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Chuyên sâu" }));
    expect(screen.getByText("RRN (DE 37)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chuyên sâu" })).toHaveAttribute("aria-pressed", "true");
  });

  it("persists the choice in the browser __MCN_004_AC2", async () => {
    renderWithIntl(<ModeToggle />);
    await userEvent.click(screen.getByRole("button", { name: "Chuyên sâu" }));
    expect(localStorage.getItem("mcn.display-mode")).toContain('"mode":"expert"');
  });
});
