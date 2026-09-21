"use client";

import { useMessageLab } from "@/shared/state/message-lab";

// Distinct, high-contrast colours per segment kind — final values come from the design
// canvas (docs/01-prd.md §7.1); these are the Tailwind token names, not hardcoded hex,
// so the design pass only needs to touch tailwind config / globals.css, not this component.
function colorFor(key: string): string {
  if (key === "mti") return "bg-accent-soft text-accent";
  if (key === "primaryBitmap" || key === "secondaryBitmap") return "bg-warn-soft text-warn";
  return "bg-canvas text-ink";
}

export function RawSegments() {
  const decoded = useMessageLab((s) => s.decoded);
  const selectedKey = useMessageLab((s) => s.selectedKey);
  const select = useMessageLab((s) => s.select);
  if (!decoded) return null;
  return (
    <div className="flex flex-wrap gap-0.5 font-mono text-sm" aria-label="Raw message">
      {decoded.segments.map((seg, i) => (
        <button
          key={`${seg.key}-${i}`}
          type="button"
          aria-current={selectedKey === seg.key}
          onClick={() => select(seg.key)}
          className={`rounded px-1 py-0.5 ${colorFor(seg.key)} ${selectedKey === seg.key ? "ring-2 ring-accent" : ""}`}
        >
          {seg.text}
        </button>
      ))}
    </div>
  );
}
