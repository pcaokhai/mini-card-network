import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";

export function DeclineReasonsBreakdown({ declineReasons }: { declineReasons: Overview["declineReasons"] }) {
  const t = useTranslations("overview.declineReasons");
  const sorted = [...declineReasons].sort((a, b) => b.share - a.share);

  return (
    <div>
      <h2 className="mb-2 text-lg font-semibold">{t("heading")}</h2>
      <ul
        className="space-y-1"
        role="img"
        aria-label={`${t("heading")}: ${sorted.map((r) => `${r.label} ${Math.round(r.share * 100)}%`).join(", ")}`}
      >
        {sorted.map((reason) => (
          <li key={reason.responseCode} className="flex items-center gap-2">
            <div className="h-2 flex-1 rounded-full bg-canvas">
              <div className="h-2 rounded-full bg-accent" style={{ width: `${reason.share * 100}%` }} />
            </div>
            <span className="w-40 shrink-0 text-sm">{reason.label}</span>
            <span className="w-12 shrink-0 text-right text-sm text-muted">{Math.round(reason.share * 100)}%</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
