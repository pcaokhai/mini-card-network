"use client";

import { useSyncExternalStore } from "react";
import { duration, ease } from "@/shared/motion/tokens";

const QUERY = "(prefers-reduced-motion: reduce)";

function subscribe(onChange: () => void): () => void {
  const mql = window.matchMedia(QUERY);
  mql.addEventListener("change", onChange);
  return () => mql.removeEventListener("change", onChange);
}

export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(subscribe, () => window.matchMedia(QUERY).matches, () => false);
}

/** MCN-004-AC3: every animation asks this hook; reduced motion becomes a short fade. */
export function useMotionPreference() {
  const reduced = usePrefersReducedMotion();
  const enter = reduced
    ? { initial: { opacity: 0 }, animate: { opacity: 1 }, transition: { duration: duration.reducedFade } }
    : { initial: { opacity: 0, y: 8 }, animate: { opacity: 1, y: 0 }, transition: { duration: duration.base, ease: ease.out } };
  return { reduced, enter } as const;
}
