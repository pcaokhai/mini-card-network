import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { AuditLine } from "./AuditLine";

describe("AuditLine", () => {
  it("renders the actor and action", () => {
    render(
      <NextIntlClientProvider locale="en" messages={en}>
        <AuditLine entry={{ actor: "operator@lab", action: "CARD_BLOCKED", occurredAt: new Date().toISOString() }} />
      </NextIntlClientProvider>,
    );
    expect(screen.getByText(/operator@lab/)).toBeInTheDocument();
    expect(screen.getByText(/blocked/i)).toBeInTheDocument();
  });
});
