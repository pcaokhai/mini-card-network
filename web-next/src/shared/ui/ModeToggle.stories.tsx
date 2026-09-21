import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { expect, userEvent, within } from "storybook/test";
import { useDisplayMode } from "@/shared/state/display-mode";
import { ModeToggle } from "@/shared/ui/ModeToggle";
import { Term } from "@/shared/ui/Term";

const meta = {
  title: "Shell/ModeToggle",
  component: ModeToggle,
  render: () => (
    <div className="flex flex-col gap-4 p-6">
      <ModeToggle />
      <p>
        <Term id="rrn" /> · <Term id="stan" /> · <Term id="mti" /> · <Term id="rc" />
      </p>
    </div>
  ),
  beforeEach: () => {
    useDisplayMode.setState({ mode: "easy" });
  },
} satisfies Meta<typeof ModeToggle>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Easy: Story = {};

export const SwitchToExpert: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Chuyên sâu" }));
    await expect(canvas.getByText("RRN (DE 37)")).toBeInTheDocument();
  },
};
