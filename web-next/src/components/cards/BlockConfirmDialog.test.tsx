import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi as vitestVi } from "vitest";
import en from "../../../messages/en.json";
import { BlockConfirmDialog } from "./BlockConfirmDialog";

describe("BlockConfirmDialog", () => {
  it("requires an inline confirm click before calling onConfirm", async () => {
    const user = userEvent.setup();
    const onConfirm = vitestVi.fn();
    render(
      <NextIntlClientProvider locale="en" messages={en}>
        <BlockConfirmDialog action="block" onConfirm={onConfirm} onCancel={vitestVi.fn()} />
      </NextIntlClientProvider>,
    );
    expect(onConfirm).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /confirm/i }));
    expect(onConfirm).toHaveBeenCalledWith("CUSTOMER_REQUEST");
  });
});
