import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { CardVisual } from "./CardVisual";

function renderVisual() {
  return render(
    <NextIntlClientProvider locale="en" messages={en}>
      <CardVisual card={{ maskedPan: "970436******4417", holderName: "Nguyen Van A", status: "ACTIVE", expiry: "09/28" }} />
    </NextIntlClientProvider>,
  );
}

describe("CardVisual", () => {
  it("renders masked PAN, holder name, and status badge", () => {
    renderVisual();
    expect(screen.getByText("970436******4417")).toBeInTheDocument();
    expect(screen.getByText("Nguyen Van A")).toBeInTheDocument();
    expect(screen.getByText(/active/i)).toBeInTheDocument();
  });
});
