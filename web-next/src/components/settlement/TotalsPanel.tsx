import { useTranslations } from "next-intl";
import type { SettlementDay } from "@/shared/api/settlement-client";
import { formatCount, formatTotal, hasReached } from "./settlement-model";

/** MCN-705-AC1: both sides' totals with the API's match badge per row (Ruling R6, R7). */
export function TotalsPanel({ day, expert }: { day: SettlementDay; expert: boolean }) {
  const t = useTranslations("settlement.totals");
  const currency = day.netPosition?.currency ?? "704";
  const { matchedCount, totalCount } = day;
  const counted = hasReached(day.stage, "RECONCILED") && matchedCount != null && totalCount ? { matchedCount, totalCount } : null;
  const ratio = counted ? Math.round((counted.matchedCount / counted.totalCount) * 1000) / 1000 : 0;
  const side = expert ? "Expert" : "Easy";

  return (
    <section aria-label={t("label")} className="set-card set-totals">
      <div className="set-card__head">
        <h2 className="set-card__title">{t(`title${side}`)}</h2>
        <span className="set-meta">
          {counted && t("matched", { matched: formatCount(counted.matchedCount), total: formatCount(counted.totalCount) })}
        </span>
      </div>
      {day.totals.length === 0 ? (
        <p className="set-empty">{t("empty")}</p>
      ) : (
        <>
          <div className="set-meter">
            <div className="set-meter__fill" style={{ transform: `scaleX(${ratio})` }} />
          </div>
          <table className="set-table">
            <colgroup>
              <col />
              <col className="set-col-value" />
              <col className="set-col-value" />
              <col className="set-col-compare" />
            </colgroup>
            <thead>
              <tr>
                <th scope="col">{t("colMetric")}</th>
                <th scope="col">{t(`colAcquirer${side}`)}</th>
                <th scope="col">{t(`colIssuer${side}`)}</th>
                <th scope="col">{t("colCompare")}</th>
              </tr>
            </thead>
            <tbody>
              {day.totals.map((row) => (
                <tr key={row.metric}>
                  <th scope="row">
                    {/* Non-breaking around "·" so a wrapped label keeps its field number with its last word. */}
                    {expert ? `${t(`metric.${row.metric}.expert`)}\u00a0·\u00a0F${row.isoField}` : t(`metric.${row.metric}.easy`)}
                  </th>
                  <td>{formatTotal(row, "acquirer", currency)}</td>
                  <td>{formatTotal(row, "issuer", currency)}</td>
                  <td>
                    <span className="set-badge" data-tone={row.matches ? "ok" : "bad"}>
                      {t(row.matches ? "match" : "mismatch")}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </section>
  );
}
