import { useTranslations } from "next-intl";
import type { ChaosScenarioId } from "@/shared/api/chaos-client";
import { SCENARIO_ICONS } from "./chaos-model";

interface ScenarioCardProps {
  id: ChaosScenarioId;
  enabled: boolean;
  /** False when the stack can't run it (ChaosScenario.available, CHA-G11); absent means available. */
  available?: boolean;
  expert: boolean;
  busy: boolean;
  onToggle: (id: ChaosScenarioId, enabled: boolean) => void;
}

/** One failure switch; the whole card turns amber while the scenario is on. */
export function ScenarioCard({ id, enabled, available = true, expert, busy, onToggle }: ScenarioCardProps) {
  const t = useTranslations("chaos");
  const titleId = `chaos-card-${id}`;
  return (
    <article className="chaos-card" data-testid={`scenario-card-${id}`} data-on={enabled} data-available={available} aria-labelledby={titleId}>
      <div className="chaos-card__head">
        <span className="chaos-card__icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d={SCENARIO_ICONS[id]} />
          </svg>
        </span>
        <h2 id={titleId} className="chaos-card__title">
          {t(`scenarios.${id}.title`)}
        </h2>
      </div>
      <p className="chaos-card__desc">{t(`scenarios.${id}.desc`)}</p>
      {expert && <div className="chaos-card__tech">{t(`scenarios.${id}.tech`)}</div>}
      {!available && <p className="chaos-card__unavailable">{t("card.unavailable")}</p>}
      <button
        type="button"
        className="chaos-card__toggle"
        aria-pressed={enabled}
        aria-busy={busy}
        disabled={!available}
        onClick={() => onToggle(id, !enabled)}
      >
        {enabled ? t("card.on") : t("card.off")}
      </button>
    </article>
  );
}
