import Link from "next/link";
import { useTranslations } from "next-intl";

export const JOURNEY_VIEWS = ["approved", "declined", "reversed"] as const;
export type JourneyView = (typeof JOURNEY_VIEWS)[number];

export const VIEW_STATUS = { approved: "APPROVED", declined: "DECLINED", reversed: "REVERSED" } as const;
/** "Đã tự hủy" means reversed by the gateway after a timeout, not a cancellation or a MAC failure (JRN-G7). */
export const VIEW_REVERSAL_REASON = { reversed: "TIMEOUT" } as const;

interface JourneyHeaderProps {
  /** The tab whose newest transaction is on screen; none when a specific RRN was opened. */
  active?: JourneyView;
  onPick: (view: JourneyView) => void;
}

/** Breadcrumb, title and the canvas's scenario tabs, here "the newest transaction of each outcome". */
export function JourneyHeader({ active, onPick }: JourneyHeaderProps) {
  const t = useTranslations("journey");
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <nav aria-label="Breadcrumb">
          <ol className="flex gap-1 text-[13px] text-muted">
            <li>
              <Link href="/" className="text-accent no-underline hover:text-[#08545a]">
                {t("breadcrumbHome")}
              </Link>
            </li>
            <li aria-hidden="true">/</li>
            <li aria-current="page">{t("title")}</li>
          </ol>
        </nav>
        <h1 className="mt-1 text-[30px] font-bold tracking-[-0.01em]">{t("title")}</h1>
      </div>
      <div role="group" aria-label={t("tabs.label")} className="flex gap-0.5 rounded-[10px] bg-[#EFEDE6] p-[3px]">
        {JOURNEY_VIEWS.map((view) => (
          <button
            key={view}
            type="button"
            aria-pressed={active === view}
            onClick={() => onPick(view)}
            className={
              active === view
                ? "h-8 rounded-lg bg-surface px-3.5 text-[13px] font-semibold text-ink shadow-[0_1px_2px_rgba(23,24,28,0.14)]"
                : "h-8 rounded-lg px-3.5 text-[13px] font-medium text-muted"
            }
          >
            {t(`tabs.${view}`)}
          </button>
        ))}
      </div>
    </div>
  );
}
