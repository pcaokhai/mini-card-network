"use client";

import { motion } from "motion/react";
import { useMotionPreference } from "@/shared/motion/useMotionPreference";
import { useMessageLab } from "@/shared/state/message-lab";

// BigInt literals (`1n`) need ES2020+; this project targets ES2017, so use BigInt(1) instead.
function bitsOf(hex: string): boolean[] {
  const value = BigInt(`0x${hex}`);
  return Array.from({ length: 64 }, (_, i) => (value & (BigInt(1) << BigInt(63 - i))) !== BigInt(0));
}

/** MCN-104-AC2: 8x8 grid, one cell per bit; page="secondary" only renders when bit 1 is set. */
export function BitmapGrid({ page }: { page: "primary" | "secondary" }) {
  const decoded = useMessageLab((s) => s.decoded);
  const selectedKey = useMessageLab((s) => s.selectedKey);
  const select = useMessageLab((s) => s.select);
  const { enter } = useMotionPreference();
  if (!decoded) return null;
  const hex = page === "primary" ? decoded.primaryBitmap : decoded.secondaryBitmap;
  if (!hex) return null;
  const bits = bitsOf(hex);
  const offset = page === "primary" ? 0 : 64;

  return (
    <div role="grid" aria-label={`${page} bitmap`} className="grid grid-cols-8 gap-1">
      {bits.map((set, i) => {
        const de = offset + i + 1;
        const key = de === 1 ? "primaryBitmap" : String(de);
        return (
          <motion.button
            key={de}
            type="button"
            role="button"
            aria-label={`bit ${de}`}
            aria-pressed={set}
            aria-current={selectedKey === key}
            onClick={() => select(key)}
            initial={enter.initial}
            animate={enter.animate}
            transition={enter.transition}
            className={`flex h-8 w-8 items-center justify-center rounded text-xs ${
              set ? "bg-accent text-white" : "bg-canvas text-muted"
            } ${selectedKey === key ? "ring-2 ring-accent" : ""}`}
          >
            {set ? "1" : "0"}
          </motion.button>
        );
      })}
    </div>
  );
}
