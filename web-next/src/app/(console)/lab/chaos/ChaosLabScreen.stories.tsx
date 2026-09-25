import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CHAOS_SCENARIO_IDS, type ChaosScenario } from "@/shared/api/chaos-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import { ChaosLabScreen } from "./ChaosLabScreen";

// Seeded caches so no story hits the network: the canvas's opening state ("Mất câu trả lời" on).
function scenarios(activeIds: string[]): ChaosScenario[] {
  return CHAOS_SCENARIO_IDS.map((id) => ({ id, enabled: activeIds.includes(id), easyText: id, technicalText: id }));
}

function seeded(activeIds: string[]) {
  return function Decorator(Story: () => React.ReactElement) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity, refetchInterval: false } } });
    client.setQueryData(["chaos", "scenarios"], scenarios(activeIds));
    client.setQueryData(["network", "saf"], { depth: 0, deadCount: 0, items: [] });
    client.setQueryData(["metrics", "overview"], { p99LatencyMs: 212 });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };
}

const meta: Meta<typeof ChaosLabScreen> = {
  component: ChaosLabScreen,
  title: "Chaos/ChaosLabScreen",
};
export default meta;
type Story = StoryObj<typeof ChaosLabScreen>;

const easy = () => {
  useDisplayMode.setState({ mode: "easy" });
};
const expert = () => {
  useDisplayMode.setState({ mode: "expert" });
};

export const Easy: Story = { decorators: [seeded(["DROP_RESPONSE"])], beforeEach: easy };
export const Expert: Story = { decorators: [seeded(["DROP_RESPONSE"])], beforeEach: expert };
export const AllCalm: Story = { decorators: [seeded([])], beforeEach: easy };
export const SeveralActive: Story = { decorators: [seeded(["SLOW_NETWORK", "CONNECTION_CUT", "ISSUER_DOWN"])], beforeEach: expert };
