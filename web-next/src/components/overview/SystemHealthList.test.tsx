import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { SystemHealthList, type SystemHealthListProps } from "./SystemHealthList";

const NOW = Date.parse("2026-09-21T14:32:20Z");

const CLOSED_SWITCH = {
  circuit: "CLOSED",
  stipActive: false,
  stipLimit: { amount: 500_000, currency: "704" },
  stipApprovedCount: 0,
} as const;

const healthy: SystemHealthListProps = {
  links: [
    {
      linkId: "gw-1",
      from: "ACQUIRER",
      to: "ISSUER",
      status: "SIGNED_ON",
      inFlight: 0,
      lastEchoAt: "2026-09-21T14:32:08Z",
    },
  ],
  saf: { depth: 0, deadCount: 0, items: [] },
  switchStatus: CLOSED_SWITCH,
  acquirerKeys: [{ keyType: "ZPK", kcv: "3F9A21", status: "ACTIVE", daysRemaining: 26, lifetimeDays: 90 }],
  expert: false,
  now: NOW,
};

describe("SystemHealthList", () => {
  it("MCN-306-AC1: shows link, SAF queue, stand-in and key health in plain language", () => {
    renderWithIntl(<SystemHealthList {...healthy} />);
    expect(screen.getByTestId("health-link")).toHaveTextContent("Ổn định · kiểm tra 12 giây trước");
    expect(screen.getByTestId("health-saf")).toHaveTextContent("Trống, không có lệnh nào chờ");
    expect(screen.getByTestId("health-stip")).toHaveTextContent("Đang tắt vì hệ thống chính hoạt động tốt");
    expect(screen.getByTestId("health-key")).toHaveTextContent("Còn hiệu lực, đổi khóa sau 26 ngày");
  });

  it("MCN-306-AC3: Expert mode shows the technical detail for each row", () => {
    renderWithIntl(<SystemHealthList {...healthy} expert />);
    expect(screen.getByTestId("health-link")).toHaveTextContent("SIGNED_ON · echo 0800/301 12s trước");
    expect(screen.getByTestId("health-saf")).toHaveTextContent("SAF depth 0 · 0 dead");
    expect(screen.getByTestId("health-stip")).toHaveTextContent("STIP OFF · circuit CLOSED");
    expect(screen.getByTestId("health-key")).toHaveTextContent("ZPK KCV 3F9A21 · rotate T-26d");
  });

  it("flags a dead SAF item, an open circuit and a near-expiry key", () => {
    renderWithIntl(
      <SystemHealthList
        {...healthy}
        saf={{ depth: 2, deadCount: 1, items: [] }}
        switchStatus={{ ...CLOSED_SWITCH, circuit: "OPEN", stipActive: true }}
        acquirerKeys={[{ keyType: "ZPK", kcv: "3F9A21", status: "ACTIVE", daysRemaining: 3, lifetimeDays: 90 }]}
      />,
    );
    expect(screen.getByTestId("health-saf")).toHaveAttribute("data-status", "bad");
    expect(screen.getByTestId("health-stip")).toHaveAttribute("data-status", "bad");
    expect(screen.getByTestId("health-key")).toHaveAttribute("data-status", "warn");
  });
});
