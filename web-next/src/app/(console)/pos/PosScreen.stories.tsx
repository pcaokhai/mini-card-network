import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import en from "../../../../messages/en.json";
import { PosScreen } from "./PosScreen";

const meta: Meta<typeof PosScreen> = {
  component: PosScreen,
  title: "POS/PosScreen",
  decorators: [
    (Story) => {
      const client = new QueryClient();
      return (
        <QueryClientProvider client={client}>
          <NextIntlClientProvider locale="en" messages={en}>
            <Story />
          </NextIntlClientProvider>
        </QueryClientProvider>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof PosScreen>;

export const Default: Story = {};
