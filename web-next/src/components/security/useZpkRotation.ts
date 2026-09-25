"use client";

import { useState } from "react";
import { useRotation, useStartRotation } from "@/shared/api/security-client";

export type RotationPhase = "idle" | "running" | "completed" | "failed";

/** The one rotation this screen offers (the canvas rotates only the ZPK), driven by the rotation resource. */
export function useZpkRotation() {
  const [rotationId, setRotationId] = useState<string | null>(null);
  const start = useStartRotation();
  const { data: rotation, isError: lostTrack } = useRotation(rotationId);

  let phase: RotationPhase = "idle";
  if (lostTrack || rotation?.status === "FAILED") phase = "failed";
  else if (start.isPending || rotation?.status === "RUNNING") phase = "running";
  else if (rotation?.status === "COMPLETED") phase = "completed";

  return {
    rotation,
    phase,
    startFailed: start.isError,
    lostTrack,
    start: () => start.mutate("ZPK", { onSuccess: (r) => setRotationId(r.rotationId) }),
    // The canvas's "Hoàn tất · làm lại" returns to the idle view; it does not rotate again.
    reset: () => {
      setRotationId(null);
      start.reset();
    },
  };
}
