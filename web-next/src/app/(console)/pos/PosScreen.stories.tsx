import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DISPLAY_CARDS } from "@/components/pos/pos-model";
import { useDisplayMode } from "@/shared/state/display-mode";
import { PosScreen } from "./PosScreen";

// Fixture balances from contracts/fixtures/cards.json, seeded so no story hits the network.
const BALANCES = [5_000_000, 80_000, 2_000_000, 1_500_000, 50_000_000, 200_000_000];

function withSeededCards(Story: () => React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  DISPLAY_CARDS.forEach((card, i) =>
    client.setQueryData(["cards", card.cardRef], {
      card: { availableBalance: { amount: BALANCES[i] ?? 0, currency: "704" } },
      etag: null,
    }),
  );
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

const meta: Meta<typeof PosScreen> = {
  component: PosScreen,
  title: "POS/PosScreen",
  decorators: [withSeededCards],
};
export default meta;
type Story = StoryObj<typeof PosScreen>;

export const Easy: Story = {
  beforeEach: () => {
    useDisplayMode.setState({ mode: "easy" });
  },
};

export const Expert: Story = {
  beforeEach: () => {
    useDisplayMode.setState({ mode: "expert" });
  },
};
