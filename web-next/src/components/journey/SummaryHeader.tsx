import { useTranslations } from "next-intl";
import { OUTCOME_TONE, type Outcome, type Tone } from "./journey-model";
import "./journey.css";

const ICON_PATH: Record<Tone, string> = {
  ok: "M5 12l5 5 9-10",
  bad: "M6 6l12 12M18 6L6 18",
  rev: "M4 12a8 8 0 1 0 3-6.2M4 4v5h5",
  warn: "M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
};

export interface SummaryContent {
  headline: string;
  meta: string;
  total: string;
  totalLabel: string;
  messages: number;
  messagesLabel: string;
  money: string;
}

/** The canvas's summary strip: outcome, then total time, message count and the customer's money. */
export function SummaryHeader({ outcome, content }: { outcome: Outcome; content: SummaryContent }) {
  const t = useTranslations("journey.summary");
  const tone = OUTCOME_TONE[outcome];
  // The customer never loses money on a reversal, so only a pending result reads as a warning.
  const moneyTone = tone === "warn" ? "warn" : "ok";
  return (
    <section
      aria-label={t("label")}
      className="grid items-center gap-5 rounded-card border border-border bg-surface px-[22px] py-[18px] md:grid-cols-[minmax(0,1.4fr)_repeat(3,minmax(0,1fr))]"
    >
      <div className="flex items-center gap-3.5">
        <div className="journey-icon h-[46px] w-[46px]" data-tone={tone}>
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d={ICON_PATH[tone]} />
          </svg>
        </div>
        <div className="flex min-w-0 flex-col gap-0.5">
          <div className="text-lg font-bold">{content.headline}</div>
          <div className="text-[13px] text-muted">{content.meta}</div>
        </div>
      </div>
      <Stat label={content.totalLabel} value={content.total} />
      <Stat label={content.messagesLabel} value={String(content.messages)} />
      <div className="journey-stat">
        <div className="text-xs text-muted">{t("money")}</div>
        <div className="text-xl font-bold" data-tone={moneyTone} style={{ color: "var(--tone-fg)" }}>
          {content.money}
        </div>
      </div>
    </section>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="journey-stat">
      <div className="text-xs text-muted">{label}</div>
      <div className="text-xl font-bold tabular-nums">{value}</div>
    </div>
  );
}
