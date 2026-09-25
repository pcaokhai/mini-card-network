import { useTranslations } from "next-intl";
import type { ReconBreak, SettlementDay } from "@/shared/api/settlement-client";
import { formatMoney } from "@/shared/format/money";
import { hasReached } from "./settlement-model";

interface BreaksPanelProps {
  day: SettlementDay;
  breaks: ReconBreak[];
  expert: boolean;
  onResolve: (item: ReconBreak) => void;
  resolvingId: string | null;
}

/** MCN-705-AC1: the reconciliation's breaks, each with its resolve action (Ruling R8). */
export function BreaksPanel({ day, breaks, expert, onResolve, resolvingId }: BreaksPanelProps) {
  const t = useTranslations("settlement.breaks");
  const reconciled = hasReached(day.stage, "RECONCILED");
  let meta = "";
  if (reconciled) meta = day.openBreaks > 0 ? t("open", { count: day.openBreaks }) : t("allResolved");

  return (
    <section aria-label={t("label")} className="set-card">
      <div className="set-card__head">
        <h2 className="set-card__title">{t("title")}</h2>
        <span className="set-meta">{meta}</span>
      </div>
      {reconciled ? (
        <ul className="set-break-list">
          {breaks.map((item) => (
            <BreakRow key={item.breakId} item={item} expert={expert} onResolve={onResolve} busy={resolvingId === item.breakId} />
          ))}
        </ul>
      ) : (
        <p className="set-empty">{t("empty")}</p>
      )}
    </section>
  );
}

interface BreakRowProps {
  item: ReconBreak;
  expert: boolean;
  onResolve: (item: ReconBreak) => void;
  busy: boolean;
}

function BreakRow({ item, expert, onResolve, busy }: BreakRowProps) {
  const t = useTranslations("settlement.breaks");
  const resolved = item.resolution !== "OPEN";
  const description = expert ? (item.technicalText ?? item.easyText) : (item.easyText ?? item.technicalText);
  return (
    <li className="set-break" data-resolved={resolved ? "" : undefined}>
      <div className="set-break__head">
        <div className="set-break__id">
          <span className="set-badge" data-tone={resolved ? "info" : "bad"}>
            {expert ? item.breakType : t(`type.${item.breakType}`)}
          </span>
          <span className="set-break__ref">{t(expert ? "refExpert" : "refEasy", { rrn: item.rrn })}</span>
        </div>
        <span className="set-break__amount">{formatMoney(item.amountDiff)}</span>
      </div>
      {description && <p className="set-break__desc">{description}</p>}
      <div>
        {resolved ? (
          <span className="set-badge" data-tone="ok">
            {t(`done.${item.breakType}`)}
          </span>
        ) : (
          <button type="button" className="set-secondary" disabled={busy} aria-busy={busy} onClick={() => onResolve(item)}>
            {t(`action.${item.breakType}`)}
          </button>
        )}
      </div>
    </li>
  );
}
