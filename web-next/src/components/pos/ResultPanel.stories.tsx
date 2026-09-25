import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import type { Transaction } from "@/shared/api/pos-client";
import { ResultPanel } from "./ResultPanel";
import "./pos.css";

const approved: Transaction = {
  rrn: "626807000301",
  stan: "000301",
  type: "PURCHASE",
  status: "APPROVED",
  responseCode: "00",
  authCode: "A00301",
  amount: { amount: 250_000, currency: "704" },
  maskedPan: "970436******4417",
  terminalId: "00000042",
  merchantName: "Cà phê Góc Phố",
  createdAt: "2026-09-25T07:00:00Z",
};
const tx = (patch: Partial<Transaction>) => ({
  kind: "tx" as const,
  tx: { ...approved, ...patch },
  last4: "4417",
  entry: "chip" as const,
  balance: 4_750_000,
});

const meta: Meta<typeof ResultPanel> = {
  component: ResultPanel,
  title: "POS/ResultPanel",
  decorators: [
    (Story) => (
      <div className="pos-root" style={{ width: 640 }}>
        <Story />
      </div>
    ),
  ],
  args: { processing: false, result: null, expert: false },
};
export default meta;
type Story = StoryObj<typeof ResultPanel>;

export const Empty: Story = {};
export const Processing: Story = { args: { processing: true } };
export const ProcessingExpert: Story = { args: { processing: true, expert: true } };
export const Approved: Story = { args: { result: tx({}) } };
export const ApprovedExpert: Story = { args: { result: tx({}), expert: true } };
export const Declined: Story = { args: { result: tx({ status: "DECLINED", responseCode: "51", authCode: null }) } };
export const TimedOut: Story = { args: { result: tx({ status: "TIMED_OUT", responseCode: null, authCode: null }) } };
export const Reversed: Story = {
  args: { result: tx({ status: "REVERSED", responseCode: null, authCode: null }), expert: true },
};
