"use client";

import { useTranslations } from "next-intl";

export type TransactionType = "PURCHASE" | "PREAUTH" | "COMPLETION" | "REFUND" | "BALANCE";

const TYPES: TransactionType[] = ["PURCHASE", "PREAUTH", "COMPLETION", "REFUND", "BALANCE"];

interface TransactionTypeSelectorProps {
  value: TransactionType;
  onChange: (type: TransactionType) => void;
}

export function TransactionTypeSelector({ value, onChange }: TransactionTypeSelectorProps) {
  const t = useTranslations("pos.transactionType");
  return (
    <div role="radiogroup" aria-label={t("label")} className="flex flex-wrap gap-2">
      {TYPES.map((type) => (
        <button
          key={type}
          type="button"
          role="radio"
          aria-checked={value === type}
          onClick={() => onChange(type)}
          className={
            value === type
              ? "rounded-full border-2 border-accent bg-accent-soft px-3 py-1.5 text-sm font-semibold"
              : "rounded-full border border-border bg-surface px-3 py-1.5 text-sm hover:bg-canvas"
          }
        >
          {t(type)}
        </button>
      ))}
    </div>
  );
}
