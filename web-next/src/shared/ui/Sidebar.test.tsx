import { screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { Sidebar } from "@/shared/ui/Sidebar";

vi.mock("next/navigation", () => ({ usePathname: () => "/lab/message" }));

describe("Sidebar", () => {
  it("shows nine entries in three groups __MCN_004_AC1", () => {
    renderWithIntl(<Sidebar />);
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(within(nav).getAllByRole("link")).toHaveLength(9);
    for (const group of ["Giao dịch", "Học và thử nghiệm", "Quản lý"]) {
      expect(within(nav).getByRole("heading", { name: group })).toBeInTheDocument();
    }
  });

  it("marks only the current route as active __MCN_004_AC1", () => {
    renderWithIntl(<Sidebar />);
    const current = screen.getAllByRole("link").filter((a) => a.getAttribute("aria-current") === "page");
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveTextContent("Phòng lab message");
  });
});
