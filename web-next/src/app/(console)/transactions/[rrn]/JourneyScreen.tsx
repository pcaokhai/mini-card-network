"use client";

import { useRouter } from "next/navigation";
import { JourneyHeader } from "@/components/journey/JourneyHeader";
import { JourneyView } from "@/components/journey/JourneyView";

/** One transaction's journey, opened from the feed, the POS or search (MCN-307-AC3). */
export function JourneyScreen({ rrn }: { rrn: string }) {
  const router = useRouter();
  return (
    <div className="flex flex-col gap-5">
      <JourneyHeader onPick={(view) => router.push(`/transactions?view=${view}`)} />
      <JourneyView rrn={rrn} />
    </div>
  );
}
