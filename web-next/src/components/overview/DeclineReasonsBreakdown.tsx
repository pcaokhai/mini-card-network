import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";
import { useResultLabel } from "@/shared/i18n/useResultLabel";

type DeclineReason = Overview["declineReasons"][number];

/** The canvas shows four named reasons; the rest share one row. */
const NAMED_REASONS = 4;

/**
 * Keeps the top four coded reasons and folds the remainder (and any provider-side uncoded row)
 * into one "other" row, so the provider stays presentation-free (OVW-G5).
 */
export function foldDeclineReasons(reasons: readonly DeclineReason[], otherLabel: string): DeclineReason[] {
  const sorted = [...reasons].sort((a, b) => b.share - a.share);
  const named = sorted.filter((r) => r.responseCode !== "").slice(0, NAMED_REASONS);
  const otherShare = sorted.filter((r) => !named.includes(r)).reduce((sum, r) => sum + r.share, 0);
  return otherShare > 0 ? [...named, { responseCode: "", label: otherLabel, share: otherShare }] : named;
}

interface DeclineReasonsBreakdownProps {
  declineReasons: Overview["declineReasons"];
  expert?: boolean;
}

/** MCN-306-AC3: Expert mode appends the ISO 8583 response code to each reason. */
export function DeclineReasonsBreakdown({ declineReasons, expert = false }: DeclineReasonsBreakdownProps) {
  const t = useTranslations("overview.declineReasons");
  const label = useResultLabel();
  const sorted = foldDeclineReasons(declineReasons, t("other")).map((reason) => ({
    ...reason,
    label: reason.responseCode ? (label.forCode(reason.responseCode, reason.label) ?? reason.label) : reason.label,
  }));

  return (
    <section
      aria-label={t("heading")}
      className="flex flex-col gap-3 rounded-card border border-border bg-surface px-5.5 py-5"
    >
      <h2 className="text-[17px] font-semibold">{t("heading")}</h2>
      <div
        role="img"
        aria-label={`${t("heading")}: ${sorted.map((r) => `${r.label} ${Math.round(r.share * 100)}%`).join(", ")}`}
        className="flex flex-col gap-3"
      >
        {sorted.map((reason) => (
          <div key={`${reason.responseCode}-${reason.label}`} className="flex flex-col gap-1.5">
            <div className="flex justify-between text-[13px]">
              <span>{expert && reason.responseCode ? `${reason.label} · RC ${reason.responseCode}` : reason.label}</span>
              <span className="font-semibold tabular-nums">{Math.round(reason.share * 100)}%</span>
            </div>
            <div className="h-2 rounded-full bg-[#EFEDE6]">
              <div className="h-2 rounded-full bg-accent" style={{ width: `${reason.share * 100}%` }} />
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
