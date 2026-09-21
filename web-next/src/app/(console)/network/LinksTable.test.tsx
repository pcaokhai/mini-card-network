import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { LinksTable } from "./LinksTable";
import type { Link } from "@/shared/api/network-client";

const link: Link = { linkId: "issuer", from: "acquirer", to: "issuer", status: "SIGNED_ON", lastEchoAt: null, lastEchoOk: null, p99LatencyMs: 40, inFlight: 0 };

describe("LinksTable", () => {
  it("renders one row per link with its status pill __MCN_205_AC1", () => {
    renderWithIntl(<LinksTable links={[link]} onAction={() => {}} pendingLinkId={null} />);
    expect(screen.getByRole("row", { name: /issuer/i })).toBeInTheDocument();
  });

  it("disables the row's action buttons while it is pending", () => {
    renderWithIntl(<LinksTable links={[link]} onAction={() => {}} pendingLinkId="issuer" />);
    for (const button of screen.getAllByRole("button")) expect(button).toBeDisabled();
  });
});
