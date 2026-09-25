import type { ReactNode } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import type { Journey } from "@/shared/api/journey-client";
import { formatOffset } from "./journey-model";
import type { StepCopy } from "./useJourneyCopy";
import "./journey.css";

type Step = Journey["steps"][number];
type Field = NonNullable<Step["message"]>["fields"][number];

interface StepDetailProps {
  step: Step;
  copy: StepCopy;
  expert: boolean;
  locale: string;
  /** Shown under the text, e.g. the accelerated countdown on a timeout step (MCN-406-AC2). */
  extra?: ReactNode;
}

/** "Chi tiết bước": who acted, what happened, and the ISO message the step carried. */
export function StepDetail({ step, copy, expert, locale, extra }: StepDetailProps) {
  const t = useTranslations("journey.detail");
  return (
    // Keyed by the parent on the step, so each step change replays the canvas's fade-up.
    <section aria-label={t("label")} className="flex animate-mcn-fade-up flex-col gap-3 rounded-card border border-border bg-surface px-[22px] py-5">
      <div className="flex items-center justify-between gap-2">
        <span className="journey-chip" data-actor={step.actor}>
          {copy.actor}
        </span>
        <span className="font-mono text-xs text-muted">{formatOffset(step.offsetMs, locale)}</span>
      </div>
      <h2 className="text-lg font-bold">{copy.title}</h2>
      <p className="text-sm leading-relaxed text-[#4a4c54]">{copy.easy}</p>
      {expert && copy.tech && (
        <div className="rounded-lg bg-canvas px-2.5 py-2 font-mono text-xs leading-normal text-[#3a3c43]">{copy.tech}</div>
      )}
      {extra}
      {step.message && <MessageFields mti={step.message.mti} fields={step.message.fields} expert={expert} />}
    </section>
  );
}

function MessageFields({ mti, fields, expert }: { mti: string; fields: Field[]; expert: boolean }) {
  const t = useTranslations("journey");
  const name = (f: Field) => (expert ? f.technicalName : t.has(`field.${f.de}`) ? t(`field.${f.de}`) : f.easyName);
  return (
    <>
      <div className="mt-1 flex items-center justify-between">
        <div className="text-[13px] font-semibold">
          {expert ? t("detail.messageExpert", { mti, count: fields.length }) : t("detail.message", { mti })}
        </div>
        <Link href="/lab/message" className="text-[13px] font-semibold text-accent hover:text-[#08545a]">
          {t("detail.openInLab")}
        </Link>
      </div>
      <div className="journey-fields" role="table" aria-label={t("detail.message", { mti })}>
        {fields.map((f) => (
          <div key={f.de} className="journey-field" role="row">
            <span role="cell" className="font-mono text-xs text-muted">
              {f.de === "MTI" ? "MTI" : expert ? `F${f.de}` : f.de}
            </span>
            <span role="cell" className="text-[13px]">
              {name(f)}
            </span>
            <span role="cell" className="truncate text-right font-mono text-xs" title={f.value}>
              {f.value}
            </span>
          </div>
        ))}
      </div>
    </>
  );
}
