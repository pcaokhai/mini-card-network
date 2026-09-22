import type { components } from "@/shared/api/generated/schema";

export type Money = components["schemas"]["Money"];

/** Money.amount is integer minor units (docs/10 §1); render as major units for display only. */
export function formatMoney(money: Money): string {
  return new Intl.NumberFormat("vi-VN", { style: "currency", currency: "VND", maximumFractionDigits: 0 }).format(
    money.amount / 100,
  );
}
