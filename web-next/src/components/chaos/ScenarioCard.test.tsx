import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { ScenarioCard } from "@/components/chaos/ScenarioCard";

describe("ScenarioCard", () => {
  it("offers to turn an off scenario on and reports the new state __MCN_405_AC1", () => {
    const onToggle = vi.fn();
    renderWithIntl(<ScenarioCard id="SLOW_NETWORK" enabled={false} expert={false} busy={false} onToggle={onToggle} />);

    const button = screen.getByRole("button", { name: "Bật sự cố" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText("Mạng chậm")).toBeInTheDocument();
    expect(screen.getByText("Mỗi yêu cầu bị trễ thêm khoảng 3 giây trên đường truyền.")).toBeInTheDocument();
    fireEvent.click(button);

    expect(onToggle).toHaveBeenCalledWith("SLOW_NETWORK", true);
  });

  it("highlights an active card and offers to turn it off __MCN_405_AC1", () => {
    const onToggle = vi.fn();
    renderWithIntl(<ScenarioCard id="DROP_RESPONSE" enabled expert={false} busy={false} onToggle={onToggle} />);

    const button = screen.getByRole("button", { name: "Đang bật · bấm để tắt" });
    expect(button).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("scenario-card-DROP_RESPONSE")).toHaveAttribute("data-on", "true");
    fireEvent.click(button);
    expect(onToggle).toHaveBeenCalledWith("DROP_RESPONSE", false);
  });

  it("shows the technical line only in expert mode __MCN_405_AC1", () => {
    const { unmount } = renderWithIntl(<ScenarioCard id="DROP_RESPONSE" enabled expert={false} busy={false} onToggle={vi.fn()} />);
    expect(screen.queryByText("Drop 0210 → timeout → 0420")).not.toBeInTheDocument();
    unmount();

    renderWithIntl(<ScenarioCard id="DROP_RESPONSE" enabled expert busy={false} onToggle={vi.fn()} />);
    expect(screen.getByText("Drop 0210 → timeout → 0420")).toBeInTheDocument();
  });

  it("disables a scenario this stack cannot run and says why __CHA_G11", () => {
    const onToggle = vi.fn();
    renderWithIntl(<ScenarioCard id="DROP_RESPONSE" enabled={false} available={false} expert={false} busy={false} onToggle={onToggle} />);

    const button = screen.getByRole("button", { name: "Bật sự cố" });
    expect(button).toBeDisabled();
    expect(screen.getByTestId("scenario-card-DROP_RESPONSE")).toHaveAttribute("data-available", "false");
    expect(screen.getByText("Môi trường này chưa chạy được sự cố này.")).toBeInTheDocument();
    fireEvent.click(button);
    expect(onToggle).not.toHaveBeenCalled();
  });
});
