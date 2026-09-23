import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { components } from "@/shared/api/generated/schema";
import "./journey.css";

type MoneyRow = components["schemas"]["Journey"]["money"][number];

export function MoneyPanel({ money, currency }: { money: MoneyRow[]; currency: string }) {
  const t = useTranslations("journey.money");
  return (
    <section aria-labelledby="journey-money-heading" className="rounded-card border border-border bg-surface p-4">
      <h2 id="journey-money-heading" className="mb-2 text-sm font-semibold">
        {t("heading")}
      </h2>
      <ul className="space-y-1 text-sm">
        {money.map((row, index) => (
          <li
            key={`${row.label}-${index}`}
            data-sign={row.delta >= 0 ? "credit" : "debit"}
            className="flex items-center justify-between gap-4"
          >
            <span>{row.label}</span>
            <span className="font-mono">
              {row.delta >= 0 ? "+" : ""}
              {formatMoney({ amount: row.delta, currency })}
              {row.balanceAfter != null && ` · ${formatMoney({ amount: row.balanceAfter, currency })}`}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
