"use client";

import { animate, motion, useMotionValue, useTransform } from "motion/react";
import { useEffect } from "react";
import { duration, ease } from "@/shared/motion/tokens";
import { usePrefersReducedMotion } from "@/shared/motion/useMotionPreference";

interface CountUpProps {
  value: number;
  format: (value: number) => string;
}

/**
 * docs/02 §7.9 "KPI count-up": the number counts from its previous value to the new one.
 * Screen readers get the settled value only, so they never announce intermediate numbers.
 */
export function CountUp({ value, format }: CountUpProps) {
  const reduced = usePrefersReducedMotion();
  const shown = useMotionValue(reduced ? value : 0);
  const text = useTransform(shown, format);

  useEffect(() => {
    if (reduced) {
      shown.set(value);
      return;
    }
    const controls = animate(shown, value, { duration: duration.travel, ease: [...ease.out] });
    return () => controls.stop();
  }, [value, reduced, shown]);

  return (
    <>
      <motion.span aria-hidden>{text}</motion.span>
      <span className="sr-only">{format(value)}</span>
    </>
  );
}
