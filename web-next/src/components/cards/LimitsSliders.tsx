"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { formatMoney, type Money } from "@/shared/format/money";
import type { CardLimits } from "@/shared/api/cards-client";

function UsageBar({ used, limit }: { used: number; limit: number }) {
  const ratio = limit > 0 ? Math.min(used / limit, 1) : 0;
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-canvas">
      <div
        className="usage-bar-fill h-full w-full rounded-full bg-accent"
        style={{ transform: `scaleX(${ratio})` }}
      />
    </div>
  );
}

export function LimitsSliders({
  limits,
  usedToday,
  onSave,
}: {
  limits: CardLimits;
  usedToday: Money;
  onSave: (newLimits: CardLimits) => void;
}) {
  const t = useTranslations("cards.limits");
  const [daily, setDaily] = useState(String(limits.dailyAmount.amount));
  const [perTransaction, setPerTransaction] = useState(String(limits.perTransactionAmount.amount));

  function handleSave() {
    onSave({
      dailyAmount: { amount: Number(daily), currency: limits.dailyAmount.currency },
      perTransactionAmount: { amount: Number(perTransaction), currency: limits.perTransactionAmount.currency },
      dailyCount: limits.dailyCount,
    });
  }

  return (
    <div className="space-y-4">
      <div>
        <label htmlFor="limits-daily" className="mb-1 block text-xs text-muted">
          {t("daily")}
        </label>
        <UsageBar used={usedToday.amount} limit={Number(daily)} />
        <p className="mt-1 text-xs text-muted">
          {formatMoney(usedToday)} / {formatMoney({ amount: Number(daily), currency: limits.dailyAmount.currency })}
        </p>
        <input
          id="limits-daily"
          type="text"
          inputMode="numeric"
          value={daily}
          onChange={(e) => setDaily(e.target.value.replace(/\D/g, ""))}
          className="mt-1 w-40 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
        />
      </div>
      <div>
        <label htmlFor="limits-per-transaction" className="mb-1 block text-xs text-muted">
          {t("perTransaction")}
        </label>
        <input
          id="limits-per-transaction"
          type="text"
          inputMode="numeric"
          value={perTransaction}
          onChange={(e) => setPerTransaction(e.target.value.replace(/\D/g, ""))}
          className="w-40 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
        />
      </div>
      <button
        type="button"
        onClick={handleSave}
        className="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-white"
      >
        {t("save")}
      </button>
    </div>
  );
}
