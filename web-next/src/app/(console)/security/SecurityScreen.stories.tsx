import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { mockSecurityKeys, resetSecurityMock } from "@/mocks/pages/security";
import { useDisplayMode } from "@/shared/state/display-mode";
import { SecurityScreen } from "./SecurityScreen";

// The dev:mock canvas keys, seeded so no story hits the network.
function withSeededKeys(Story: () => React.ReactElement) {
  resetSecurityMock();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["security", "keys", "acquirer"], mockSecurityKeys());
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

const meta: Meta<typeof SecurityScreen> = {
  component: SecurityScreen,
  title: "Security/SecurityScreen",
  decorators: [withSeededKeys],
};
export default meta;
type Story = StoryObj<typeof SecurityScreen>;

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
