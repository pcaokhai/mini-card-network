import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";
import { useResultLabel } from "@/shared/i18n/useResultLabel";

interface DeclineReasonsBreakdownProps {
  declineReasons: Overview["declineReasons"];
  expert?: boolean;
}

/** MCN-306-AC3: Expert mode appends the ISO 8583 response code to each reason. */
export function DeclineReasonsBreakdown({ declineReasons, expert = false }: DeclineReasonsBreakdownProps) {
  const t = useTranslations("overview.declineReasons");
  const label = useResultLabel();
  const sorted = [...declineReasons]
    .sort((a, b) => b.share - a.share)
    .map((reason) => ({ ...reason, label: label.forCode(reason.responseCode, reason.label) ?? reason.label }));

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
