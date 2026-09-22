import { useTranslations } from "next-intl";
import { formatMoney, type Money } from "@/shared/format/money";
import { HoldsList, type Hold } from "@/components/cards/HoldsList";

export function BalanceLines({
  ledgerBalance,
  availableBalance,
  holds,
}: {
  ledgerBalance: Money;
  availableBalance: Money;
  holds: Hold[];
}) {
  const t = useTranslations("cards.balance");
  const heldTotal = holds
    .filter((h) => h.status === "ACTIVE")
    .reduce((sum, h) => sum + h.amount.amount, 0);
  return (
    <dl className="grid grid-cols-3 gap-4">
      <div>
        <dt className="text-xs text-muted">{t("ledger")}</dt>
        <dd className="text-lg font-semibold">{formatMoney(ledgerBalance)}</dd>
      </div>
      <div>
        <dt className="text-xs text-muted">{t("available")}</dt>
        <dd className="text-lg font-semibold">{formatMoney(availableBalance)}</dd>
      </div>
      <div>
        <dt className="text-xs text-muted">{t("held")}</dt>
        <dd className="text-lg font-semibold">{formatMoney({ amount: heldTotal, currency: ledgerBalance.currency })}</dd>
      </div>
      <div className="col-span-3">
        <HoldsList holds={holds} />
      </div>
    </dl>
  );
}
