import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { components } from "@/shared/api/generated/schema";
import { SETTLEMENT_BREAKS, settlementDayAt } from "@/mocks/pages/settlement";
import { SettlementApiError } from "@/shared/api/settlement-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import { SettlementScreen } from "./SettlementScreen";

type SettlementStage = components["schemas"]["SettlementStage"];
const BUSINESS_DATE = "2026-09-21";
const RESOLVED = SETTLEMENT_BREAKS.map((b) => ({ ...b, resolution: "MANUAL_ADJUSTED" as const }));

// Seeded with the dev:mock canvas values so no story hits the network.
function seeded(stage: SettlementStage | "unavailable") {
  return function WithSeededDay(Story: () => React.ReactElement) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false, staleTime: Infinity } } });
    if (stage === "unavailable") {
      client
        .getQueryCache()
        .build(client, { queryKey: ["settlement", "day", BUSINESS_DATE] })
        .setState({ status: "error", error: new SettlementApiError(404, "404 page not found"), fetchStatus: "idle" });
    } else {
      const breaks = stage === "FILE_GENERATED" ? RESOLVED : SETTLEMENT_BREAKS;
      client.setQueryData(["settlement", "day", BUSINESS_DATE], settlementDayAt(BUSINESS_DATE, stage, breaks));
      client.setQueryData(["settlement", "breaks", BUSINESS_DATE], breaks);
    }
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };
}

const easy = () => {
  useDisplayMode.setState({ mode: "easy" });
};
const expert = () => {
  useDisplayMode.setState({ mode: "expert" });
};

const meta: Meta<typeof SettlementScreen> = {
  component: SettlementScreen,
  title: "Settlement/SettlementScreen",
  args: { businessDate: BUSINESS_DATE },
};
export default meta;
type Story = StoryObj<typeof SettlementScreen>;

export const Easy: Story = { decorators: [seeded("TOTALS_EXCHANGED")], beforeEach: easy };
export const Expert: Story = { decorators: [seeded("TOTALS_EXCHANGED")], beforeEach: expert };
export const OpenDay: Story = { decorators: [seeded("OPEN")], beforeEach: easy };
export const ReconciledWithBreaks: Story = { decorators: [seeded("RECONCILED")], beforeEach: easy };
export const ReconciledExpert: Story = { decorators: [seeded("RECONCILED")], beforeEach: expert };
export const FileGenerated: Story = { decorators: [seeded("FILE_GENERATED")], beforeEach: easy };
export const FileGeneratedExpert: Story = { decorators: [seeded("FILE_GENERATED")], beforeEach: expert };
export const Unavailable: Story = { decorators: [seeded("unavailable")], beforeEach: easy };
