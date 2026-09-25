import { useTranslations } from "next-intl";
import type { JournalEntry } from "@/shared/api/cards-client";
import { formatMoney } from "@/shared/format/money";
import { type LedgerRow, ledgerRow } from "./cards-model";

const clock = new Intl.DateTimeFormat("vi-VN", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
const plain = new Intl.NumberFormat("vi-VN");
const CURRENCY = "704";

function sign(effect: number) {
  return effect < 0 ? "out" : effect > 0 ? "in" : "none";
}

interface LedgerPanelProps {
  entries: readonly JournalEntry[];
  expert: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
}

export function LedgerPanel({ entries, expert, hasMore, loadingMore, onLoadMore }: LedgerPanelProps) {
  const t = useTranslations("cards.ledger");
  const mode = expert ? "expert" : "easy";

  function legs(row: LedgerRow): [string, string] {
    if (expert) {
      return [
        row.debitAccount === null ? "—" : t("debit", { account: row.debitAccount }),
        row.creditAccount === null ? t("notPosted") : t("credit", { account: row.creditAccount, amount: plain.format(row.amount) }),
      ];
    }
    const amount = formatMoney({ amount: Math.abs(row.effect), currency: CURRENCY });
    return [t(`kind.${sign(row.effect)}`), row.effect < 0 ? `−${amount}` : row.effect > 0 ? `+${amount}` : amount];
  }

  return (
    <section aria-labelledby="cards-ledger-heading" className="cards-panel cards-ledger" data-mode={mode}>
      <div className="cards-ledger__head">
        <h2 id="cards-ledger-heading" className="cards-panel__heading">
          {t(`${mode}.title`)}
        </h2>
        <span className="cards-ledger__note">{t(`${mode}.note`)}</span>
      </div>
      <div role="table" aria-labelledby="cards-ledger-heading" className="cards-ledger__table">
        <div role="row" data-head="" className="cards-ledger__row">
          <span role="columnheader">{t("time")}</span>
          <span role="columnheader">{t("description")}</span>
          <span role="columnheader">{t(`${mode}.a`)}</span>
          <span role="columnheader">{t(`${mode}.b`)}</span>
        </div>
        {entries.map(ledgerRow).map((row) => {
          const [a, b] = legs(row);
          return (
            <div role="row" key={row.journalId} className="cards-ledger__row">
              <span role="cell" className="cards-ledger__time">
                {clock.format(new Date(row.occurredAt))}
              </span>
              <span role="cell" className="cards-ledger__desc">
                {row.description ?? t(`type.${row.entryType}`)}
              </span>
              <span role="cell" className="cards-ledger__a">
                {a}
              </span>
              <span role="cell" className="cards-ledger__b" data-sign={sign(row.effect)}>
                {b}
              </span>
            </div>
          );
        })}
      </div>
      {entries.length === 0 && <p className="cards-ledger__empty">{t("empty")}</p>}
      {hasMore && (
        <button type="button" className="cards-ledger__more" disabled={loadingMore} onClick={onLoadMore}>
          {t(`${mode}.more`)}
        </button>
      )}
    </section>
  );
}
