import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi as vitestVi } from "vitest";
import en from "../../../messages/en.json";
import { LimitsSliders } from "./LimitsSliders";

function renderSliders(usedToday: number, onSave = vitestVi.fn()) {
  render(
    <NextIntlClientProvider locale="en" messages={en}>
      <LimitsSliders
        limits={{ dailyAmount: { amount: 1000000, currency: "704" }, perTransactionAmount: { amount: 500000, currency: "704" } }}
        usedToday={{ amount: usedToday, currency: "704" }}
        onSave={onSave}
      />
    </NextIntlClientProvider>,
  );
  return onSave;
}

describe("LimitsSliders", () => {
  it("shows a usage bar proportional to usedToday/dailyAmount", () => {
    renderSliders(250000);
    const bar = document.querySelector(".usage-bar-fill");
    expect(bar).toHaveStyle({ transform: "scaleX(0.25)" });
  });

  it("calls onSave with the new limit values", async () => {
    const user = userEvent.setup();
    const onSave = renderSliders(0);
    const dailyInput = screen.getByLabelText(/daily limit/i);
    await user.clear(dailyInput);
    await user.type(dailyInput, "2000000");
    await user.click(screen.getByRole("button", { name: /save/i }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ dailyAmount: { amount: 2000000, currency: "704" } }));
  });
});
