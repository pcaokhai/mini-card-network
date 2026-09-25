import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import type { Journey } from "@/shared/api/journey-client";
import { formatOffset, KIND_TONE } from "./journey-model";
import type { StepCopy } from "./useJourneyCopy";
import "./journey.css";

type Step = Journey["steps"][number];

interface StepTimelineProps {
  steps: Step[];
  copy: (step: Step, index: number) => StepCopy;
  locale: string;
  currentStep: number;
  onSelectStep: (index: number) => void;
  controls: ReactNode;
}

/** "Từng bước, theo thời gian": every step up to the current one lit, later ones dimmed. */
export function StepTimeline({ steps, copy, locale, currentStep, onSelectStep, controls }: StepTimelineProps) {
  const t = useTranslations("journey.steps");
  return (
    <section aria-label={t("label")} className="flex flex-col gap-1.5 rounded-card border border-border bg-surface px-4 py-[18px]">
      {/* As in the canvas, the heading gives way (wraps) before the controls do. */}
      <div className="flex flex-wrap items-center justify-between gap-3 px-1.5 pb-2.5 min-[1400px]:flex-nowrap">
        <h2 className="min-w-0 text-[17px] font-semibold">{t("heading")}</h2>
        <div className="shrink-0">{controls}</div>
      </div>
      <ol className="flex flex-col gap-1.5">
        {steps.map((step, index) => {
          const c = copy(step, index);
          const current = index === currentStep;
          return (
            <li key={step.seq}>
              <button
                type="button"
                className="journey-step"
                aria-current={current ? "step" : undefined}
                data-future={index > currentStep}
                data-kind={step.kind}
                onClick={() => onSelectStep(index)}
              >
                <span className="pt-[3px] font-mono text-xs text-muted">{formatOffset(step.offsetMs, locale)}</span>
                <span className="flex justify-center pt-1">
                  <span className="journey-dot" data-tone={KIND_TONE[step.kind]} data-current={current} />
                </span>
                <span className="flex min-w-0 flex-col gap-1">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="journey-chip" data-actor={step.actor}>
                      {c.actor}
                    </span>
                    <span className="text-sm font-semibold">{c.title}</span>
                  </span>
                  <span className="text-[13px] leading-normal text-[#4a4c54]">{c.line}</span>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
