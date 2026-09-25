import { useTranslations } from "next-intl";
import { STEP_KEYS, stageIndex, stepStatus, type SettlementStage, type StepStatus, dayLabels } from "./settlement-model";

const TONE: Record<StepStatus, "ok" | "warn" | "info"> = { done: "ok", current: "warn", pending: "info" };

interface StageStepsProps {
  stage: SettlementStage;
  expert: boolean;
  dates: ReturnType<typeof dayLabels>;
  /** A failed action's problem detail; a new `key` re-runs the shake. */
  error: { text: string; key: number } | null;
}

/** MCN-705-AC1/AC2: the four steps driven by `SettlementDay.stage`, and the refused action's detail. */
export function StageSteps({ stage, expert, dates, error }: StageStepsProps) {
  const t = useTranslations("settlement.steps");
  const justDone = stageIndex(stage) - 1;
  return (
    <section aria-label={t("label")} className="set-card">
      <ol className="set-steps__grid">
        {STEP_KEYS.map((key, i) => {
          const status = stepStatus(stage, i);
          return (
            <li key={key} className="set-step" data-status={status} aria-current={status === "current" ? "step" : undefined}>
              <div className="set-step__head">
                <span className="set-step__num" data-pop={i === justDone ? "" : undefined}>
                  {i + 1}
                </span>
                <span className="set-badge" data-tone={TONE[status]}>
                  {t(`status.${status}`)}
                </span>
              </div>
              <div className="set-step__title">{t(`${key}.title`, dates)}</div>
              <p className="set-step__desc">{t(`${key}.${expert ? "expert" : "easy"}`, dates)}</p>
            </li>
          );
        })}
      </ol>
      {error && (
        <div role="alert" key={error.key} className="set-alert set-shake">
          {error.text}
        </div>
      )}
    </section>
  );
}
