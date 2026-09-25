import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { JourneyScreen } from "./JourneyScreen";
import type { Journey } from "@/shared/api/journey-client";
import { APPROVED_JOURNEY, AUTO_REVERSED_JOURNEY, DECLINED_JOURNEY, MOCK_CARDS, MOCK_LEDGERS } from "@/mocks/journey-fixtures";

function withJourney(journey: Journey) {
  const { rrn, maskedPan } = journey.transaction;
  const card = MOCK_CARDS.find((c) => c.maskedPan === maskedPan);
  return function Decorator(Story: () => React.ReactElement) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    client.setQueryData(["transactions", rrn, "journey"], journey);
    client.setQueryData(["transactions", rrn, "customer-ledger"], card ? { currentBalance: card.balance, newestFirst: MOCK_LEDGERS[card.cardRef] ?? [] } : null);
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };
}

const meta: Meta<typeof JourneyScreen> = {
  component: JourneyScreen,
  title: "Transactions/JourneyScreen",
  decorators: [withJourney(APPROVED_JOURNEY)],
  args: { rrn: APPROVED_JOURNEY.transaction.rrn },
  // The tabs navigate with the App Router.
  parameters: { nextjs: { appDirectory: true } },
};
export default meta;
type Story = StoryObj<typeof JourneyScreen>;

export const Approved: Story = {};

export const AutoReversed: Story = {
  args: { rrn: AUTO_REVERSED_JOURNEY.transaction.rrn },
  decorators: [withJourney(AUTO_REVERSED_JOURNEY)],
};

export const Declined: Story = {
  args: { rrn: DECLINED_JOURNEY.transaction.rrn },
  decorators: [withJourney(DECLINED_JOURNEY)],
};
