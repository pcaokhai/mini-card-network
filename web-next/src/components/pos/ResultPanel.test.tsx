import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { components } from "@/shared/api/generated/schema";
import { ResultPanel } from "./ResultPanel";

type Transaction = components["schemas"]["Transaction"];

const approved: Transaction = {
  rrn: "x",
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

const declined: Transaction = {
  ...approved,
  stan: "000123",
  status: "DECLINED",
  responseCode: "62",
  responseLabel: "Card is blocked",
  maskedPan: "970436******3310",
};

describe("ResultPanel", () => {
  it("shows a plain outcome in easy mode with no MTI/STAN/RC line", () => {
    render(<ResultPanel transaction={approved} expertMode={false} />);
    expect(screen.getByText(/approved/i)).toBeInTheDocument();
    expect(screen.queryByText(/RC /i)).not.toBeInTheDocument();
  });

  it("shows the MTI/STAN/RC line in expert mode", () => {
    render(<ResultPanel transaction={declined} expertMode={true} />);
    expect(screen.getByText(/RC 62/)).toBeInTheDocument();
  });

  it("applies the decline animation class for a DECLINED status", () => {
    const { container } = render(<ResultPanel transaction={declined} expertMode={false} />);
    expect(container.querySelector('[data-outcome="DECLINED"]')).toBeInTheDocument();
  });
});
