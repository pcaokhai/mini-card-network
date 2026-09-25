import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { SystemHealthList } from "./SystemHealthList";
import type { Overview } from "@/shared/api/overview-client";
import type { Link } from "@/shared/api/network-client";

const overview: Overview = {
  transactionsToday: 1,
  approvalRate: 1,
  p99LatencyMs: 1,
  ledgerMatches: true,
  throughput: [],
  declineReasons: [],
};

const links: Link[] = [
  { linkId: "gw-1", from: "ACQUIRER", to: "ISSUER", status: "SIGNED_ON", inFlight: 0 },
];

describe("SystemHealthList", () => {
  it("shows ledger health and one row per link", () => {
    renderWithIntl(<SystemHealthList overview={overview} links={links} />);
    expect(screen.getByTestId("ledger-health-status")).toHaveAttribute("data-status", "ok");
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("flags ledger mismatch", () => {
    renderWithIntl(<SystemHealthList overview={{ ...overview, ledgerMatches: false }} links={[]} />);
    expect(screen.getByTestId("ledger-health-status")).toHaveAttribute("data-status", "warn");
  });
});
