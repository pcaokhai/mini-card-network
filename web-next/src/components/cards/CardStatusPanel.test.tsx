import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { mockCardDetail } from "@/mocks/pages/cards";
import type { CardDetail } from "@/shared/api/cards-client";
import { CardStatusPanel } from "./CardStatusPanel";

const base = mockCardDetail("crd_normal0001") as CardDetail;

describe("CardStatusPanel", () => {
  it.each(["LOST", "STOLEN", "PIN_BLOCKED"] as const)("offers no unblock for a %s card, which the issuer refuses CARDS-G5", (status) => {
    renderWithIntl(<CardStatusPanel card={{ ...base, status }} kind="locked" expert={false} audit={[]} pending={false} error={null} onConfirm={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "Mở khóa thẻ" })).not.toBeInTheDocument();
    expect(screen.getByText("Thẻ báo mất, bị đánh cắp hoặc khóa PIN không mở khóa được. Khách cần được phát hành thẻ mới.")).toBeInTheDocument();
  });

  it("still offers unblock for a card an operator blocked CARDS-G5", () => {
    renderWithIntl(<CardStatusPanel card={{ ...base, status: "BLOCKED" }} kind="locked" expert={false} audit={[]} pending={false} error={null} onConfirm={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Mở khóa thẻ" })).toBeInTheDocument();
  });
});
