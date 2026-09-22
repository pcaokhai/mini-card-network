import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ChaosLabScreen } from "./ChaosLabScreen";
import { CHAOS_SCENARIO_IDS, type ChaosScenario } from "@/shared/api/chaos-client";

function scenarios(activeIds: string[]): ChaosScenario[] {
  return CHAOS_SCENARIO_IDS.map((id) => ({
    id,
    enabled: activeIds.includes(id),
    easyText: id.replace(/_/g, " ").toLowerCase(),
    technicalText: `${id} technical description`,
  }));
}

function withQueryClient(scenarioData: ChaosScenario[]) {
  return function Decorator(Story: () => React.ReactElement) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    client.setQueryData(["chaos", "scenarios"], scenarioData);
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

export const NoActiveScenarios: Story = {
  decorators: [withQueryClient(scenarios([]))],
};

export const SeveralActive: Story = {
  decorators: [withQueryClient(scenarios(["SLOW_NETWORK", "ISSUER_DOWN"]))],
};
