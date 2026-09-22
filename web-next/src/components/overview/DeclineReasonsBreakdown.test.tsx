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
});
