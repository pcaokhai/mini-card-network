import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { usePathname } from "next/navigation";
import { Sidebar } from "@/shared/ui/Sidebar";

const meta = {
  title: "Shell/Sidebar",
  component: Sidebar,
  // @storybook/nextjs-vite 10.6's `parameters.nextjs.navigation.pathname` isn't wired for
  // addon-vitest's headless browser runner (its mock only spies on the real usePathname,
  // which returns null outside a live Next.js router). Override the mock's return value
  // directly instead — it's a storybook/test `fn()` under this framework's Vite alias.
  beforeEach: () => {
    (usePathname as unknown as { mockReturnValue: (v: string) => void }).mockReturnValue("/lab/message");
  },
} satisfies Meta<typeof Sidebar>;
export default meta;

export const MessageLabActive: StoryObj<typeof meta> = {};
