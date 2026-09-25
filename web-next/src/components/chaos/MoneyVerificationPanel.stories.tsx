import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import type { ChaosRun } from "@/shared/api/chaos-client";
import { MoneyVerificationPanel } from "./MoneyVerificationPanel";
import "./chaos.css";

const passed: ChaosRun = {
  runId: "run-1",
  status: "PASSED",
  requested: 100,
  completed: 100,
  approved: 86,
  declined: 6,
  reversed: 8,
  openingBalanceTotal: 500_000_000,
  closingBalanceTotal: 484_090_000,
  ledgerDiscrepancy: 0,
};

const meta: Meta<typeof MoneyVerificationPanel> = {
  component: MoneyVerificationPanel,
  title: "Chaos/MoneyVerificationPanel",
  decorators: [
    (Story) => (
      <div className="chaos-root" style={{ width: 420 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof MoneyVerificationPanel>;

export const NotRun: Story = { args: { run: undefined, expert: false } };
export const Running: Story = { args: { run: { ...passed, status: "RUNNING", completed: 40, approved: 35, reversed: 3, closingBalanceTotal: 0 }, expert: false } };
export const Passed: Story = { args: { run: passed, expert: false } };
export const PassedExpert: Story = { args: { run: passed, expert: true } };
export const Discrepancy: Story = { args: { run: { ...passed, status: "FAILED", ledgerDiscrepancy: 185_000 }, expert: false } };
