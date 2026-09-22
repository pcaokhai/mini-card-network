import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { LimitsSliders } from "./LimitsSliders";

const meta: Meta<typeof LimitsSliders> = { component: LimitsSliders, title: "Cards/LimitsSliders" };
export default meta;
type Story = StoryObj<typeof LimitsSliders>;

export const Default: Story = {
  args: {
    limits: { dailyAmount: { amount: 1000000, currency: "704" }, perTransactionAmount: { amount: 500000, currency: "704" } },
    usedToday: { amount: 250000, currency: "704" },
    onSave: () => {},
  },
};

export const NearLimit: Story = {
  args: {
    limits: { dailyAmount: { amount: 1000000, currency: "704" }, perTransactionAmount: { amount: 500000, currency: "704" } },
    usedToday: { amount: 950000, currency: "704" },
    onSave: () => {},
  },
};
