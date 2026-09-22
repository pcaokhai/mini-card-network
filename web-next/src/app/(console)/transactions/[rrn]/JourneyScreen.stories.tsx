import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { JourneyScreen } from "./JourneyScreen";
import type { Journey } from "@/shared/api/journey-client";

const SAMPLE_JOURNEY: Journey = {
  transaction: {
    rrn: "123456789012",
    status: "APPROVED",
    type: "PURCHASE",
    responseCode: "00",
    responseLabel: "Approved",
    amount: { amount: 10000, currency: "704" },
    maskedPan: "970436******4417",
    terminalId: "00000042",
    merchantName: "Ca phe Goc Pho",
    createdAt: new Date().toISOString(),
  },
  steps: [
    { seq: 0, actor: "POS", offsetMs: 0, title: "Purchase requested", easyText: "The card was tapped at the register.", technicalText: "0200 built and sent.", kind: "INFO", message: null },
    { seq: 1, actor: "ISSUER", offsetMs: 50, title: "Approved", easyText: "The bank approved the purchase.", technicalText: "0210 RC 00 returned.", kind: "OK", message: null },
    { seq: 2, actor: "ACQUIRER", offsetMs: 60, title: "Response returned", easyText: "The register shows approved.", technicalText: "0210 relayed to POS.", kind: "INFO", message: null },
  ] as Journey["steps"],
  money: [{ label: "Purchase", delta: -10000, balanceAfter: 490000, atStep: 1 }],
};

function withQueryClient(Story: () => React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["transactions", "123456789012", "journey"], SAMPLE_JOURNEY);
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

const meta: Meta<typeof JourneyScreen> = {
  component: JourneyScreen,
  title: "Transactions/JourneyScreen",
  decorators: [withQueryClient],
  args: { rrn: "123456789012" },
};
export default meta;
type Story = StoryObj<typeof JourneyScreen>;

export const Default: Story = {};
