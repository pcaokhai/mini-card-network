import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { EventTimeline } from "./EventTimeline";

describe("EventTimeline", () => {
  it("renders events newest first with severity distinguished __MCN_205_AC2", () => {
    renderWithIntl(
      <EventTimeline
        events={[
          { id: "1", occurredAt: "2026-09-22T07:00:00Z", severity: "ERROR", easyText: "Link went down", technicalText: "3 timeouts" },
          { id: "2", occurredAt: "2026-09-22T08:00:00Z", severity: "INFO", easyText: "Link is healthy", technicalText: "SIGNED_ON" },
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Link is healthy");
    expect(items[0]).toHaveAttribute("data-severity", "INFO");
  });

  it("shows an empty state with no events", () => {
    renderWithIntl(<EventTimeline events={[]} />);
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });
});
