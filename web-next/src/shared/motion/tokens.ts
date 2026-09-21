/** Motion tokens (docs/02 §7.9). Durations are seconds because Motion uses seconds. */
export const duration = { instant: 0.1, fast: 0.18, base: 0.26, slow: 0.42, travel: 0.75, reducedFade: 0.12 } as const;
export const ease = { out: [0.22, 1, 0.36, 1], in: [0.4, 0, 1, 1] } as const;
export const spring = {
  snappy: { type: "spring", stiffness: 500, damping: 32 },
  soft: { type: "spring", stiffness: 220, damping: 26 },
} as const;
export const stagger = { step: 0.04, maxItems: 8 } as const;
