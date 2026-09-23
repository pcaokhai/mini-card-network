import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { PinBlockVisualiser } from "./PinBlockVisualiser";

function renderVisualiser() {
  return render(
    <NextIntlClientProvider locale="en" messages={en}>
      <PinBlockVisualiser />
    </NextIntlClientProvider>,
  );
}

describe("PinBlockVisualiser", () => {
  it("labels itself as illustration only__MCN_505_AC2", () => {
    renderVisualiser();
    expect(screen.getByText(/illustration only/i)).toBeInTheDocument();
  });

  it("computes the format-0 block for a valid 4-digit PIN__MCN_505_AC2", async () => {
    renderVisualiser();
    await userEvent.type(screen.getByRole("textbox", { name: /pin/i }), "1234");
    expect(screen.getByTestId("pin-block-hex")).toHaveTextContent(/^[0-9A-F]{16}$/);
  });

  it("shows an inline error for a PIN outside 4-12 digits__MCN_505_AC2", async () => {
    renderVisualiser();
    await userEvent.type(screen.getByRole("textbox", { name: /pin/i }), "123");
    expect(screen.getByRole("alert")).toHaveTextContent(/4.*12/);
  });
});
