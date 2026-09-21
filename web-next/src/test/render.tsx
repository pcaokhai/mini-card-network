import { render } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import type { ReactElement } from "react";
import vi from "../../messages/vi.json";

export function renderWithIntl(ui: ReactElement) {
  return render(
    <NextIntlClientProvider locale="vi" messages={vi}>
      {ui}
    </NextIntlClientProvider>,
  );
}
