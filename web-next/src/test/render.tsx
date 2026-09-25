import { render } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import type { ReactElement } from "react";
import vi from "../../messages/vi.json";

export function renderWithIntl(ui: ReactElement) {
  // Retries off so a component whose query has no handler fails fast instead of hanging the test.
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <NextIntlClientProvider locale="vi" messages={vi}>
        {ui}
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}
