import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";
import en from "../../../messages/en.json";
import { TransactionTypeSelector } from "./TransactionTypeSelector";

function renderSelector(onChange: (type: string) => void) {
  return render(
    <NextIntlClientProvider locale="en" messages={en}>
      <TransactionTypeSelector value="PURCHASE" onChange={onChange} />
    </NextIntlClientProvider>,
  );
}

describe("TransactionTypeSelector", () => {
  it("renders all five types and calls onChange on selection", async () => {
    const onChange = vi.fn();
    renderSelector(onChange);

    expect(screen.getByRole("radio", { name: /purchase/i })).toHaveAttribute("aria-checked", "true");
    await userEvent.click(screen.getByRole("radio", { name: /pre-auth/i }));

    expect(onChange).toHaveBeenCalledWith("PREAUTH");
    expect(screen.getAllByRole("radio")).toHaveLength(5);
  });
});
