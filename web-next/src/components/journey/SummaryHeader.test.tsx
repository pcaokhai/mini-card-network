import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/test/render";
import type { components } from "@/shared/api/generated/schema";
import { SummaryHeader } from "./SummaryHeader";

const transaction: components["schemas"]["Transaction"] = {
  rrn: "123456789012",
  status: "APPROVED",
  type: "PURCHASE",
  responseCode: "00",
  responseLabel: "Approved",
  amount: { amount: 10000, currency: "704" },
  maskedPan: "970436******4417",
  terminalId: "00000042",
  merchantName: "Ca phe Goc Pho",
  createdAt: new Date().toISOString(),
};

describe("SummaryHeader", () => {
  it("renders RRN, status, amount and masked PAN", () => {
    renderWithIntl(<SummaryHeader transaction={transaction} />);
    expect(screen.getByText("123456789012")).toBeInTheDocument();
    expect(screen.getByText(/Approved/)).toBeInTheDocument();
    expect(screen.getByText(/10\.000\s₫/)).toBeInTheDocument();
    expect(screen.getByText("970436******4417")).toBeInTheDocument();
  });
});
