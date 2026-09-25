import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import { DeclineReasonsBreakdown } from "./DeclineReasonsBreakdown";

describe("DeclineReasonsBreakdown", () => {
  it("renders each decline reason with its share, labelled in the console's locale by RC", () => {
    // The gateway sends English labels; the console labels known codes itself.
    renderWithIntl(
      <DeclineReasonsBreakdown
        declineReasons={[
          { responseCode: "51", label: "Insufficient funds", share: 0.6 },
          { responseCode: "62", label: "Card is blocked", share: 0.4 },
        ]}
      />,
    );
    expect(screen.getByText("Không đủ tiền")).toBeInTheDocument();
    expect(screen.getByText("Thẻ bị khóa")).toBeInTheDocument();
    expect(screen.getByText(/60%/)).toBeInTheDocument();
  });

  it("falls back to the server label for a code the RC table does not know", () => {
    renderWithIntl(
      <DeclineReasonsBreakdown declineReasons={[{ responseCode: "Q1", label: "Issuer-specific", share: 1 }]} />,
    );
    expect(screen.getByText("Issuer-specific")).toBeInTheDocument();
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
