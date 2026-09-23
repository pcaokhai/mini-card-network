import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../messages/en.json";
import { PciNeverDoList } from "./PciNeverDoList";

function renderList(mode: "easy" | "expert") {
  return render(
    <NextIntlClientProvider locale="en" messages={en}>
      <PciNeverDoList mode={mode} />
    </NextIntlClientProvider>,
  );
}

describe("PciNeverDoList", () => {
  it("renders the never-do items in Expert wording__MCN_505_AC3", () => {
    renderList("expert");
    expect(screen.getByText(/never.*log.*clear key/i)).toBeInTheDocument();
  });

  it("renders the never-do items in Easy wording__MCN_505_AC3", () => {
    renderList("easy");
    expect(screen.getByRole("list")).toBeInTheDocument();
  });
});
