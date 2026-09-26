import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import type { KeyInfo } from "@/shared/api/security-client";
import { KeyTable } from "./KeyTable";

const zpk = (kcv: string, status: KeyInfo["status"]): KeyInfo => ({ keyType: "ZPK", kcv, status, daysRemaining: 20, lifetimeDays: 30 });

describe("KeyTable", () => {
  it("lists an ACTIVE and a PENDING key of the same type as two rows and offers rotation only on the active one __SEC_G8", () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    renderWithIntl(<KeyTable keys={[zpk("AABBCC", "ACTIVE"), zpk("DDEEFF", "PENDING")]} expert={false} canRotate onRotate={vi.fn()} newKcv={null} />);

    expect(screen.getByText("AABBCC")).toBeInTheDocument();
    expect(screen.getByText("DDEEFF")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Xoay khóa ngay" })).toHaveLength(1);
    expect(errors).not.toHaveBeenCalledWith(expect.stringContaining("same key"), expect.anything(), expect.anything());
    errors.mockRestore();
  });
});
