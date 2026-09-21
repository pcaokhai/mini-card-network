import type { Preview } from "@storybook/nextjs-vite";
import { NextIntlClientProvider } from "next-intl";
import vi from "../messages/vi.json";
import "../src/app/globals.css";

const preview: Preview = {
  decorators: [
    (Story) => (
      <NextIntlClientProvider locale="vi" messages={vi}>
        <Story />
      </NextIntlClientProvider>
    ),
  ],
  parameters: {
    // MCN-004: accessibility violations fail story tests (docs/08 TS-17).
    a11y: { test: "error" },
  },
};

export default preview;
