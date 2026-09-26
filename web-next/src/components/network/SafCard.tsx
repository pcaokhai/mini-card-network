import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { SafQueue } from "@/shared/api/network-client";

type SafItem = SafQueue["items"][number];

// Visible rows only; the badge carries the full depth (web-next/CLAUDE.md caps realtime lists at 30).
const MAX_ROWS = 30;

function useItemTitle(expert: boolean) {
  const t = useTranslations("network.saf");
  return (item: SafItem) => {
    const isReversal = item.mti === "0420";
    if (expert) return t(isReversal ? "reversalTech" : "adviceTech", { mti: item.mti });
    const label = t(isReversal ? "reversal" : "advice");
    return item.amount ? t("withAmount", { label, amount: formatMoney(item.amount) }) : label;
  };
}

/** MCN-804-AC1: the store-and-forward queue the SAF worker is still delivering. */
/** `failed` renders the read error in place of the queue, so a gateway outage never looks like an empty queue (NET-G11). */
export function SafCard({ saf, expert, failed = false }: { saf: SafQueue | undefined; expert: boolean; failed?: boolean }) {
  const t = useTranslations("network.saf");
  const itemTitle = useItemTitle(expert);
  const depth = saf?.depth ?? 0;
  const dead = saf?.deadCount ?? 0;
  const tone = dead > 0 ? "bad" : depth > 0 ? "warn" : "ok";

  return (
    <section aria-label={t("ariaLabel")} className="net-card gap-2.5">
      <div className="flex items-center justify-between">
        <h2>{t(expert ? "titleTech" : "titleEasy")}</h2>
        {!failed && (
          <span className="net-badge" data-tone={tone}>
            {depth > 0 ? t("depth", { count: depth.toLocaleString("vi-VN") }) : t("emptyBadge")}
          </span>
        )}
      </div>
      {failed && (
        <p role="alert" className="net-desc net-desc--error">
          {t("loadFailed")}
        </p>
      )}
      {!failed && depth === 0 && <p className="net-desc net-desc--muted">{expert ? t("emptyTech", { depth, dead }) : t("emptyEasy")}</p>}
      {saf?.items.slice(0, MAX_ROWS).map((item) => (
        <div key={item.id} className="net-saf-item" data-status={item.status}>
          <span className="flex flex-col gap-0.5">
            <span className="net-saf-item__title">{itemTitle(item)}</span>
            <span className="net-saf-item__ref">{t("rrn", { rrn: item.rrn })}</span>
          </span>
          <span className="net-saf-item__attempts">{t(expert ? "attemptsTech" : "attemptsEasy", { n: item.attempts })}</span>
        </div>
      ))}
    </section>
  );
}
