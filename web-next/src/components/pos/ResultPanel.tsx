import Link from "next/link";
import type { CSSProperties } from "react";
import { useTranslations } from "next-intl";
import { MTI, type ResultKind, type TransactionType } from "./pos-model";
import { type PosResult, describeResult } from "./result-view";

interface ResultPanelProps {
  processing: boolean;
  result: PosResult | null;
  expert: boolean;
  type?: TransactionType;
}

export function ResultPanel({ processing, result, expert, type = "PURCHASE" }: ResultPanelProps) {
  const t = useTranslations("pos.result");
  return (
    <section aria-label={t("sectionLabel")} aria-live="polite" className="pos-panel pos-result">
      {processing ? (
        <div className="pos-result__center pos-result__center--processing">
          <span className="pos-spinner" aria-hidden="true" />
          <div className="pos-result__center-title">{t("procTitle")}</div>
          <div className="pos-result__center-desc">
            {expert ? t("procDescExpert", { request: MTI[type][0], response: MTI[type][1] }) : t("procDesc")}
          </div>
        </div>
      ) : result === null ? (
        <div className="pos-result__center">
          <EmptyCardIcon />
          <div className="pos-result__center-title">{t("emptyTitle")}</div>
          <div className="pos-result__center-desc">{t("emptyDesc")}</div>
        </div>
      ) : (
        <ResultBody result={result} expert={expert} />
      )}
    </section>
  );
}

function ResultBody({ result, expert }: { result: PosResult; expert: boolean }) {
  const t = useTranslations("pos.result");
  const view = describeResult(result, t);
  return (
    // Keyed on the view so a second result replays the pop/shake and the step stagger.
    <div key={`${view.rrn}-${view.title}`} data-kind={view.kind} className="contents">
      <div className="pos-result__head">
        <div className="pos-result__icon">
          <ResultIcon kind={view.kind} />
        </div>
        <div className="pos-result__text">
          <div className="pos-result__title">{view.title}</div>
          <div className="pos-result__desc">{view.desc}</div>
          {expert && <div className="pos-result__tech">{view.tech}</div>}
        </div>
      </div>
      {view.steps.length > 0 && (
        <ol className="pos-steps">
          {view.steps.map((step, i) => (
            <li key={step.title} className="pos-step" style={{ "--step-index": i } as CSSProperties}>
              <span className="pos-step__dot" data-kind={step.kind} />
              <div className="pos-step__text">
                <span className="pos-step__title">{step.title}</span>
                <span className="pos-step__desc">{step.desc}</span>
              </div>
            </li>
          ))}
        </ol>
      )}
      {view.rrn && (
        <Link href={`/transactions/${view.rrn}`} className="pos-result__link">
          {t("viewJourney")}
        </Link>
      )}
    </div>
  );
}

function ResultIcon({ kind }: { kind: ResultKind }) {
  const path = {
    ok: "M5 12l5 5 9-10",
    bad: "M6 6l12 12M18 6L6 18",
    rev: "M4 12a8 8 0 1 0 3-6.2M4 4v5h5",
  }[kind];
  return (
    <svg
      width="22"
      height="22"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={path} />
    </svg>
  );
}

function EmptyCardIcon() {
  return (
    <svg
      width="40"
      height="40"
      viewBox="0 0 24 24"
      fill="none"
      stroke="var(--pos-device-meta)"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="3" y="6" width="18" height="12" rx="2" />
      <path d="M3 10h18M7 15h3" />
    </svg>
  );
}
