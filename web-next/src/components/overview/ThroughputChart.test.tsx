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
});
