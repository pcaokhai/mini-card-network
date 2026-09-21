import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useMessageLab } from "@/shared/state/message-lab";
import { BitmapGrid } from "./BitmapGrid";

const decoded = { mti: "0800", primaryBitmap: "8220000000000000", secondaryBitmap: null, segments: [], fields: [] };

const meta = {
  title: "Lab/BitmapGrid",
  component: BitmapGrid,
  beforeEach: () => {
    useMessageLab.setState({ decoded, selectedKey: null, raw: "" });
  },
} satisfies Meta<typeof BitmapGrid>;
export default meta;

export const Primary: StoryObj<typeof meta> = { args: { page: "primary" } };
