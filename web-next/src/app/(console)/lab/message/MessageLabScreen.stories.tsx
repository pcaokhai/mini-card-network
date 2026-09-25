import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LAB_SAMPLES } from "@/mocks/pages/lab";
import { useDisplayMode } from "@/shared/state/display-mode";
import { MessageLabScreen } from "./MessageLabScreen";

// Seeded with the dev:mock canvas samples so no story hits the network.
function withSeededSamples(Story: () => React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["lab", "samples"], LAB_SAMPLES.map(({ mti, label, raw }) => ({ mti, label, raw })));
  LAB_SAMPLES.forEach((s) => client.setQueryData(["lab", "decode", s.raw], s.decoded));
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

const meta: Meta<typeof MessageLabScreen> = {
  component: MessageLabScreen,
  title: "Lab/MessageLabScreen",
  decorators: [withSeededSamples],
};
export default meta;
type Story = StoryObj<typeof MessageLabScreen>;

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
