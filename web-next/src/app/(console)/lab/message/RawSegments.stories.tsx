import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { useMessageLab } from "@/shared/state/message-lab";
import { RawSegments } from "./RawSegments";

const decoded = {
  mti: "0800",
  primaryBitmap: "8220000000000000",
  secondaryBitmap: null,
  segments: [
    { key: "mti", text: "0800" },
    { key: "primaryBitmap", text: "8220000000000000" },
    { key: "7", text: "0921073300" },
    { key: "11", text: "000200" },
    { key: "70", text: "301" },
  ],
  fields: [],
};

const meta = {
  title: "Lab/RawSegments",
  component: RawSegments,
  beforeEach: () => {
    useMessageLab.setState({ decoded, selectedKey: null, raw: "" });
  },
} satisfies Meta<typeof RawSegments>;
export default meta;

export const Echo: StoryObj<typeof meta> = {};
