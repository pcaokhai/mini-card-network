import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { DeclineReasonsBreakdown } from "./DeclineReasonsBreakdown";

describe("DeclineReasonsBreakdown", () => {
  it("renders each decline reason with its share", () => {
    renderWithIntl(
      <DeclineReasonsBreakdown
        declineReasons={[
          { responseCode: "51", label: "Insufficient funds", share: 0.6 },
          { responseCode: "62", label: "Blocked card", share: 0.4 },
        ]}
      />,
    );
    expect(screen.getByText(/insufficient funds/i)).toBeInTheDocument();
    expect(screen.getByText(/60%/)).toBeInTheDocument();
  });

  it("MCN-306-AC3: Expert mode appends the RC code, except for the uncoded remainder", () => {
    renderWithIntl(
      <DeclineReasonsBreakdown
        expert
        declineReasons={[
          { responseCode: "51", label: "Không đủ tiền", share: 0.94 },
          { responseCode: "", label: "Lý do khác", share: 0.06 },
        ]}
      />,
    );
    expect(screen.getByText("Không đủ tiền · RC 51")).toBeInTheDocument();
    expect(screen.getByText("Lý do khác")).toBeInTheDocument();
  });

  it("Easy mode hides RC codes", () => {
    renderWithIntl(
      <DeclineReasonsBreakdown declineReasons={[{ responseCode: "51", label: "Không đủ tiền", share: 1 }]} />,
    );
    expect(screen.queryByText(/RC 51/)).not.toBeInTheDocument();
  });
});
