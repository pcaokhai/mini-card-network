import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { LinkStatusPill } from "./LinkStatusPill";

const meta: Meta<typeof LinkStatusPill> = { component: LinkStatusPill, title: "Network/LinkStatusPill" };
export default meta;
type Story = StoryObj<typeof LinkStatusPill>;

export const SignedOn: Story = { args: { status: "SIGNED_ON" } };
export const Connected: Story = { args: { status: "CONNECTED" } };
export const Down: Story = { args: { status: "DOWN" } };
export const Disconnected: Story = { args: { status: "DISCONNECTED" } };
