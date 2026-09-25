"use client";

import { type KeyboardEvent, type PointerEvent, useState } from "react";
import { useTranslations } from "next-intl";
import { type CardDetail, PreconditionFailedError, useUpdateLimits } from "@/shared/api/cards-client";
import { formatMoney } from "@/shared/format/money";
import { LIMIT_RANGES, usagePercent } from "./cards-model";
import { problemDetail } from "./problem-detail";

type LimitKey = keyof typeof LIMIT_RANGES;
const KEYS: readonly LimitKey[] = ["daily", "perTransaction"];

interface LimitsPanelProps {
  card: CardDetail;
  etag: string | null;
  expert: boolean;
  onReload: () => void;
}

/**
 * Sliders save when released (pointer or key up), not on every step of a drag: each save carries
 * the card's ETag as If-Match, so a burst of saves would 412 against its own first write.
 */
export function LimitsPanel({ card, etag, expert, onReload }: LimitsPanelProps) {
  const t = useTranslations("cards.limits");
  const update = useUpdateLimits(card.cardRef);
  const [draft, setDraft] = useState<Partial<Record<LimitKey, number>>>({});
  const [stale, setStale] = useState(false);
  const saved: Record<LimitKey, number> = { daily: card.limits.dailyAmount.amount, perTransaction: card.limits.perTransactionAmount.amount };
  const value = (key: LimitKey) => draft[key] ?? saved[key];
  const currency = card.limits.dailyAmount.currency;
  const money = (amount: number) => formatMoney({ amount, currency });

  function commit(key: LimitKey, amount: number) {
    if (amount === saved[key]) return;
    const next = { ...saved, ...draft, [key]: amount };
    update.mutate(
      {
        limits: { dailyAmount: { amount: next.daily, currency }, perTransactionAmount: { amount: next.perTransaction, currency }, dailyCount: card.limits.dailyCount },
        ifMatch: etag,
      },
      {
        onSuccess: () => setDraft({}),
        onError: (error) => setStale(error instanceof PreconditionFailedError),
      },
    );
  }

  function reload() {
    setStale(false);
    setDraft({});
    update.reset();
    onReload();
  }

  const used = card.usedToday.amount;
  const percent = usagePercent(used, value("daily"));

  return (
    <section aria-labelledby="cards-limits-heading" className="cards-panel cards-limits">
      <h2 id="cards-limits-heading" className="cards-panel__heading">
        {t("heading")}
      </h2>
      {KEYS.map((key) => {
        const id = `cards-limit-${key}`;
        const valueText = value(key) > 0 ? money(value(key)) : t("unset");
        const release = (e: PointerEvent<HTMLInputElement> | KeyboardEvent<HTMLInputElement>) => commit(key, Number(e.currentTarget.value));
        return (
          <div key={key} className="cards-limit">
            <label htmlFor={id} className="cards-limit__label">
              <span>{t(`${expert ? "expert" : "easy"}.${key}`)}</span>
              <span>{valueText}</span>
            </label>
            <input
              id={id}
              type="range"
              className="cards-limit__slider"
              {...LIMIT_RANGES[key]}
              value={value(key)}
              aria-valuetext={valueText}
              onChange={(e) => setDraft((d) => ({ ...d, [key]: Number(e.target.value) }))}
              onPointerUp={release}
              onKeyUp={release}
            />
            {key === "daily" && (
              <>
                <div className="cards-usage">
                  <div className="cards-usage__fill" style={{ transform: `scaleX(${percent / 100})` }} />
                </div>
                <div className="cards-limit__used">
                  {value("daily") > 0 ? t("used", { used: money(used), percent }) : t("usedNoLimit", { used: money(used) })}
                </div>
              </>
            )}
          </div>
        );
      })}
      {stale && (
        <div role="alert" className="cards-stale">
          <span>{t("stale")}</span>
          <button type="button" onClick={reload}>
            {t("reload")}
          </button>
        </div>
      )}
      {update.isError && !stale && (
        <p role="alert" className="cards-error">
          {t("failed", { detail: problemDetail(update.error) })}
        </p>
      )}
    </section>
  );
}
