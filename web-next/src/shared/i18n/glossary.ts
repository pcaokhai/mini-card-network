/** Technical labels are the same in every locale; easy labels and hints live in messages/<locale>.json. */
export const TECHNICAL_LABELS = {
  rrn: "RRN (DE 37)",
  stan: "STAN (DE 11)",
  mti: "MTI",
  rc: "Response code (DE 39)",
} as const;

export type GlossaryId = keyof typeof TECHNICAL_LABELS;
