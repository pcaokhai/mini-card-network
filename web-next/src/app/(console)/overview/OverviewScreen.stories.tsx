import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { OverviewScreen } from "./OverviewScreen";
import type { Overview } from "@/shared/api/overview-client";

const SAMPLE_OVERVIEW: Overview = {
  transactionsToday: 4231,
  approvalRate: 0.94,
  p99LatencyMs: 118,
  ledgerMatches: true,
  throughput: Array.from({ length: 60 }, (_, i) => ({
    at: new Date(Date.now() - (59 - i) * 60_000).toISOString(),
    tps: Math.round(10 + Math.sin(i / 5) * 8 + (i % 7)),
  })),
  declineReasons: [
    { responseCode: "51", label: "Insufficient funds", share: 0.45 },
    { responseCode: "62", label: "Blocked card", share: 0.3 },
    { responseCode: "05", label: "Do not honor", share: 0.25 },
  ],
};

function withQueryClient(Story: () => React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["metrics", "overview"], SAMPLE_OVERVIEW);
  client.setQueryData(["network", "links"], []);
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

const meta: Meta<typeof OverviewScreen> = {
  component: OverviewScreen,
  title: "Overview/OverviewScreen",
  decorators: [withQueryClient],
};
export default meta;
type Story = StoryObj<typeof OverviewScreen>;

export const Default: Story = {};
