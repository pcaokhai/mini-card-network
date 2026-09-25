import type { components } from "@/shared/api/generated/schema";

export type Money = components["schemas"]["Money"];

// ISO 4217 numeric -> alpha map for currencies used in this project's fixtures (contracts/fixtures/cards.json).
// ponytail: extend when a new currency shows up in contracts.
const CURRENCY_ALPHA: Record<string, string> = { "704": "VND" };

const MINOR_DIGITS: Record<string, number> = { VND: 0 };
const DEFAULT_MINOR_DIGITS = 2;

/** Money.amount is integer minor units (docs/10 §1); render as major units for display only. */
export function formatMoney({ amount, currency }: Money): string {
  const code = CURRENCY_ALPHA[currency] ?? currency;
  const fractionDigits = MINOR_DIGITS[code] ?? DEFAULT_MINOR_DIGITS;
  const major = amount / 10 ** fractionDigits;

  // web-next/CLAUDE.md: amounts render through vi-VN, which is also what the design canvas shows.
  if (CURRENCY_ALPHA[currency] === undefined) {
    return `${new Intl.NumberFormat("vi-VN", {
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    }).format(major)} ${code}`;
  }

  return new Intl.NumberFormat("vi-VN", {
    style: "currency",
    currency: code,
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(major);
}
