import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { KeyLifetimeBar } from "./KeyLifetimeBar";

function renderBar(props: { daysRemaining: number; lifetimeDays: number }) {
  return render(
    <NextIntlClientProvider locale="en" messages={en}>
      <KeyLifetimeBar {...props} />
    </NextIntlClientProvider>,
  );
}

describe("KeyLifetimeBar", () => {
  it("renders a warning when daysRemaining is under 30__MCN_505_AC1", () => {
    renderBar({ daysRemaining: 12, lifetimeDays: 365 });
    expect(screen.getByRole("alert")).toHaveTextContent(/expir/i);
  });

  it("renders no warning well within lifetime__MCN_505_AC1", () => {
    renderBar({ daysRemaining: 300, lifetimeDays: 365 });
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
