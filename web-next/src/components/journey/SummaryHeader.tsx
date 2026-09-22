import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { components } from "@/shared/api/generated/schema";

type Transaction = components["schemas"]["Transaction"];

export function SummaryHeader({ transaction }: { transaction: Transaction }) {
  const t = useTranslations("overview.liveFeed");
  const isApproved = transaction.status === "APPROVED";

  return (
    <header className="flex flex-wrap items-center gap-4 rounded-card border border-border bg-surface p-4">
      <div>
        <div className="text-xs text-muted">{t("rrn")}</div>
        <div className="font-mono">{transaction.rrn}</div>
      </div>
      <div className={isApproved ? "text-lg font-semibold text-ok" : "text-lg font-semibold text-bad"}>
        {transaction.responseLabel ?? transaction.status}
      </div>
      <div className="font-mono">{formatMoney(transaction.amount)}</div>
      <div className="text-sm text-muted">{transaction.maskedPan}</div>
    </header>
  );
}
