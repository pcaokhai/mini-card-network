import { useTranslations } from "next-intl";
import type { CardDetail } from "@/shared/api/cards-client";
import { formatMoney } from "@/shared/format/money";
import { activeHoldTotal } from "./cards-model";

// "26/09" as in the canvas; vi-VN's own day-month pattern is "26-09".
const dayMonth = (iso: string) => {
  const d = new Date(iso);
  return `${String(d.getDate()).padStart(2, "0")}/${String(d.getMonth() + 1).padStart(2, "0")}`;
};

/** Three lines: what the ledger holds, what is on hold, and what is left to spend. */
export function BalancePanel({ card, expert }: { card: CardDetail; expert: boolean }) {
  const t = useTranslations("cards.balance");
  const labels = expert ? "expert" : "easy";
  const currency = card.ledgerBalance.currency;
  const held = activeHoldTotal(card.holds);
  const holds = card.holds.filter((h) => h.status === "ACTIVE");

  return (
    <section aria-labelledby="cards-balance-heading" className="cards-panel cards-balance">
      <h2 id="cards-balance-heading" className="cards-panel__heading">
        {t("heading")}
      </h2>
      <dl className="cards-balance__lines">
        <div className="cards-balance__line" data-line="ledger">
          <dt>{t(`${labels}.ledger`)}</dt>
          <dd>{formatMoney(card.ledgerBalance)}</dd>
        </div>
        <div className="cards-balance__line" data-line="held">
          <dt>{t(`${labels}.held`)}</dt>
          <dd>{held > 0 ? `−${formatMoney({ amount: held, currency })}` : formatMoney({ amount: 0, currency })}</dd>
        </div>
        <div className="cards-balance__line" data-line="available">
          <dt>{t(`${labels}.available`)}</dt>
          <dd>{formatMoney(card.availableBalance)}</dd>
        </div>
      </dl>
      {holds.length > 0 && (
        <ul className="cards-holds">
          {holds.map((hold) => (
            <li key={hold.holdId} className="cards-hold">
              <span>{t(expert ? "holdExpert" : "holdEasy", { merchant: hold.merchantName, amount: formatMoney(hold.amount) })}</span>
              <span>{t("holdUntil", { date: dayMonth(hold.expiresAt) })}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
