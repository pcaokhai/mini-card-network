import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useMessageLab } from "@/shared/state/message-lab";
import { FieldTable } from "./FieldTable";

const decoded = {
  mti: "0800",
  primaryBitmap: "0".repeat(16),
  secondaryBitmap: null,
  segments: [],
  fields: [{ de: "11", easyName: "Sequence number", technicalName: "STAN", format: "n", value: "000200" }],
};

const meta = {
  title: "Lab/FieldTable",
  component: FieldTable,
  beforeEach: () => {
    useMessageLab.setState({ decoded, selectedKey: null, raw: "" });
  },
} satisfies Meta<typeof FieldTable>;
export default meta;

export const Easy: StoryObj<typeof meta> = {};
