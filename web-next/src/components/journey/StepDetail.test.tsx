import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { StepDetail } from "./StepDetail";

const step = {
  seq: 0,
  actor: "POS",
  offsetMs: 0,
  title: "Purchase requested",
  easyText: "The terminal asked the network to approve a purchase.",
  technicalText: "0200 sent",
  kind: "INFO",
  message: {
    mti: "0200",
    fields: [
      { de: "MTI", easyName: "Message type", technicalName: "MTI", format: "n4", value: "0200" },
      { de: "3", easyName: "Processing code", technicalName: "Processing code", format: "n6", value: "000000" },
    ],
  },
} as any;

describe("StepDetail", () => {
  it("renders the step's ISO message fields", () => {
    renderWithIntl(<StepDetail step={step} />);
    expect(screen.getAllByText("MTI").length).toBeGreaterThan(0);
    expect(screen.getByText("0200")).toBeInTheDocument();
    expect(screen.getByText("Processing code")).toBeInTheDocument();
  });

  it("shows a fallback when the step has no message", () => {
    renderWithIntl(<StepDetail step={{ ...step, message: null }} />);
    expect(screen.queryByText("MTI")).not.toBeInTheDocument();
  });
});
