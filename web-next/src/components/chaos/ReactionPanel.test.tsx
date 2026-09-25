import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { ReactionPanel } from "@/components/chaos/ReactionPanel";

describe("ReactionPanel", () => {
  it("shows the calm text and easy tile labels with nothing active __MCN_405_AC3", () => {
    renderWithIntl(<ReactionPanel activeScenarios={[]} expert={false} safDepth={0} p99LatencyMs={212} />);
    expect(screen.getByText(/Mọi thứ đang bình thường/)).toBeInTheDocument();
    expect(screen.getByText("Lệnh đang chờ gửi lại")).toBeInTheDocument();
    expect(screen.getByText("Thời gian phản hồi")).toBeInTheDocument();
    expect(screen.getByText("212 ms")).toBeInTheDocument();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("lists one reaction per active scenario in canvas order with easy text __MCN_405_AC3", () => {
    renderWithIntl(
      <ReactionPanel activeScenarios={["ISSUER_DOWN", "SLOW_NETWORK"]} expert={false} safDepth={36} p99LatencyMs={3200} />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items.map((li) => within(li).getAllByText(/./)[0]?.textContent)).toEqual(["Mạng chậm", "Ngân hàng phát hành sập"]);
    expect(within(items[0] as HTMLElement).getByText(/khoảng 3,2 giây nhưng vẫn trong giới hạn chờ/)).toBeInTheDocument();
    expect(screen.queryByText(/Mọi thứ đang bình thường/)).not.toBeInTheDocument();
    expect(screen.getByText("36")).toBeInTheDocument();
    expect(screen.getByText("3,2 giây")).toBeInTheDocument();
  });

  it("switches reaction text and tile labels to the technical wording in expert mode __MCN_405_AC3", () => {
    renderWithIntl(<ReactionPanel activeScenarios={["DROP_RESPONSE"]} expert safDepth={0} p99LatencyMs={212} />);
    expect(screen.getByText("0200 timeout → 0420 field 90 → 0430")).toBeInTheDocument();
    expect(screen.getByText("SAF depth")).toBeInTheDocument();
    expect(screen.getByText("p99 end-to-end")).toBeInTheDocument();
  });

  it("shows a dash while SAF depth or latency is unknown __MCN_405_AC3", () => {
    renderWithIntl(<ReactionPanel activeScenarios={[]} expert={false} safDepth={undefined} p99LatencyMs={undefined} />);
    expect(screen.getAllByText("—")).toHaveLength(2);
  });
});
