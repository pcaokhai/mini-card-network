import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LEDGER_PAGE_SIZE } from "@/components/cards/CardDetail";
import { mockCardDetail, mockCardSummaries, mockLedger } from "@/mocks/pages/cards";
import { useDisplayMode } from "@/shared/state/display-mode";
import { CardsScreen } from "./CardsScreen";

// The dev:mock canvas data, seeded so no story hits the network.
function withSeededCards(Story: () => React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const cards = mockCardSummaries();
  client.setQueryData(["cards"], cards);
  for (const { cardRef } of cards) {
    client.setQueryData(["cards", cardRef], { card: mockCardDetail(cardRef), etag: '"v1"' });
    client.setQueryData(["cards", cardRef, "ledger", "pages", LEDGER_PAGE_SIZE], {
      pages: [{ items: mockLedger(cardRef).slice(0, LEDGER_PAGE_SIZE), nextCursor: null }],
      pageParams: [undefined],
    });
  }
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

const meta: Meta<typeof CardsScreen> = {
  component: CardsScreen,
  title: "Cards/CardsScreen",
  decorators: [withSeededCards],
};
export default meta;
type Story = StoryObj<typeof CardsScreen>;

const easy = () => {
  useDisplayMode.setState({ mode: "easy" });
};
const expert = () => {
  useDisplayMode.setState({ mode: "expert" });
};

export const Easy: Story = { beforeEach: easy };
export const Expert: Story = { beforeEach: expert };
export const BlockedEasy: Story = { args: { cardRef: "crd_blockd0003" }, beforeEach: easy };
export const BlockedExpert: Story = { args: { cardRef: "crd_blockd0003" }, beforeEach: expert };
export const ExpiredEasy: Story = { args: { cardRef: "crd_expird0004" }, beforeEach: easy };
export const DeclinesEasy: Story = { args: { cardRef: "crd_lowbal0002" }, beforeEach: easy };
