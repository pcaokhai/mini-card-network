import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { TransactionTypeSelector } from "./TransactionTypeSelector";

describe("TransactionTypeSelector", () => {
  it("MCN-604 renders the five types as an on/off segmented control", async () => {
    const onChange = vi.fn();
    renderWithIntl(<TransactionTypeSelector value="PURCHASE" onChange={onChange} />);

    const group = screen.getByRole("group", { name: "Loại giao dịch" });
    expect(group.querySelectorAll("button")).toHaveLength(5);
    expect(screen.getByRole("button", { name: "Mua hàng" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(screen.getByRole("button", { name: "Tiền ủy quyền trước" }));
    expect(onChange).toHaveBeenCalledWith("PREAUTH");
  });
});
