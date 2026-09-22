import type { components } from "@/shared/api/generated/schema";

export type Money = components["schemas"]["Money"];

// ISO 4217 numeric -> alpha map for currencies used in this project's fixtures (contracts/fixtures/cards.json).
// ponytail: extend when a new currency shows up in contracts.
const CURRENCY_ALPHA: Record<string, string> = { "704": "VND" };

/** Money.amount is integer minor units (docs/10 §1); render as major units for display only. */
export function formatMoney({ amount, currency }: Money): string {
  const code = CURRENCY_ALPHA[currency] ?? currency;
  const fractionDigits = code === "VND" ? 0 : 2;
  const major = amount / 10 ** fractionDigits;
  const formatted = new Intl.NumberFormat("en-US", {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(major);
  return `${formatted} ${code}`;
}
