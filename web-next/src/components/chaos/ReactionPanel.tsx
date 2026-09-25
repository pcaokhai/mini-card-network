import { useTranslations } from "next-intl";
import { CHAOS_SCENARIO_IDS, type ChaosScenarioId } from "@/shared/api/chaos-client";
import { latencyParts } from "./chaos-model";

interface ReactionPanelProps {
  activeScenarios: ChaosScenarioId[];
  expert: boolean;
  safDepth: number | undefined;
  p99LatencyMs: number | undefined;
}

const DASH = "—";

/** "Hệ thống đang phản ứng thế nào": live SAF depth and p99 (Ruling R4), then one reaction per active failure. */
export function ReactionPanel({ activeScenarios, expert, safDepth, p99LatencyMs }: ReactionPanelProps) {
  const t = useTranslations("chaos.reaction");
  const tScenario = useTranslations("chaos.scenarios");
  const mode = expert ? "expert" : "easy";
  const latency = p99LatencyMs === undefined ? undefined : latencyParts(p99LatencyMs);
  // Canvas order, not the order the switches were flipped.
  const active = CHAOS_SCENARIO_IDS.filter((id) => activeScenarios.includes(id));

  return (
    <section aria-labelledby="chaos-reaction-heading" className="chaos-panel chaos-reaction">
      <h2 id="chaos-reaction-heading" className="chaos-panel__heading">
        {t("heading")}
      </h2>
      <div className="chaos-tiles">
        <div className="chaos-tile">
          <div className="chaos-tile__label">{t(`${mode}.saf`)}</div>
          <div className="chaos-tile__value">{safDepth === undefined ? DASH : new Intl.NumberFormat("vi-VN").format(safDepth)}</div>
        </div>
        <div className="chaos-tile">
          <div className="chaos-tile__label">{t(`${mode}.latency`)}</div>
          <div className="chaos-tile__value">{latency ? t(`latency.${latency.unit}`, { value: latency.value }) : DASH}</div>
        </div>
      </div>
      {active.length === 0 ? (
        <p className="chaos-reaction__calm">{t("calm")}</p>
      ) : (
        <ul className="chaos-reaction__list" aria-live="polite">
          {active.map((id) => (
            <li key={id} className="chaos-reaction__item">
              <span className="chaos-reaction__dot" aria-hidden="true" />
              <div className="chaos-reaction__body">
                <span className="chaos-reaction__title">{tScenario(`${id}.title`)}</span>
                <span className="chaos-reaction__text">{tScenario(`${id}.${expert ? "reactTech" : "react"}`)}</span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
