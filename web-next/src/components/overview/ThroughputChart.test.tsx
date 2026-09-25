import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { ThroughputChart } from "./ThroughputChart";

describe("ThroughputChart", () => {
  it("renders one bar per throughput sample", () => {
    const throughput = Array.from({ length: 60 }, (_, i) => ({ at: new Date().toISOString(), tps: i }));
    renderWithIntl(<ThroughputChart throughput={throughput} />);
    expect(screen.getAllByRole("img", { hidden: true }).length + document.querySelectorAll(".throughput-bar").length).toBeGreaterThan(0);
    expect(document.querySelectorAll(".throughput-bar")).toHaveLength(60);
  });

  it("scales bar height by the max tps value, compositor-friendly (transform, not height)", () => {
    const throughput = [
      { at: "a", tps: 1 },
      { at: "b", tps: 10 },
    ];
    renderWithIntl(<ThroughputChart throughput={throughput} />);
    const bars = document.querySelectorAll(".throughput-bar");
    expect(bars[1].getAttribute("style")).toMatch(/transform/);
    expect(bars[1].getAttribute("style")).not.toMatch(/height:\s*\d/);
  });

  it("fills the chart for real per-second rates below 1, scaled to the busiest bucket", () => {
    // A lab day runs at hundredths of a transaction per second; the busiest bar must still be full.
    const throughput = [
      { at: "a", tps: 0.02 },
      { at: "b", tps: 0.04 },
    ];
    renderWithIntl(<ThroughputChart throughput={throughput} />);
    const bars = document.querySelectorAll(".throughput-bar");
    expect(bars[1].getAttribute("style")).toContain("scaleY(1)");
    expect(bars[0].getAttribute("style")).toContain("scaleY(0.5)");
  });

  it("draws empty bars, not NaN, when nothing happened", () => {
    renderWithIntl(<ThroughputChart throughput={[{ at: "a", tps: 0 }]} />);
    expect(document.querySelector(".throughput-bar")?.getAttribute("style")).toContain("scaleY(0)");
  });
});
