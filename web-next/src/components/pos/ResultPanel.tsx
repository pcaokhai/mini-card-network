import Link from "next/link";
import { useTranslations } from "next-intl";
import type { components } from "@/shared/api/generated/schema";
import "./pos.css";

type Transaction = components["schemas"]["Transaction"];

interface ResultPanelProps {
  transaction: Transaction | null;
  expertMode: boolean;
}

export function ResultPanel({ transaction, expertMode }: ResultPanelProps) {
  const t = useTranslations("journey");
  if (!transaction) return null;
  const isApproved = transaction.status === "APPROVED";

  return (
    <div
      className="result-panel rounded-card border border-border bg-surface p-4"
      data-outcome={transaction.status}
    >
      <div className={isApproved ? "text-lg font-semibold text-ok" : "text-lg font-semibold text-bad"}>
        {transaction.responseLabel ?? transaction.status}
      </div>
      <div className="text-sm text-muted">{transaction.maskedPan}</div>
      {expertMode && (
        <div className="mt-2 font-mono text-xs text-muted">
          STAN {transaction.stan ?? "—"} · RC {transaction.responseCode ?? "—"}
        </div>
      )}
      {transaction.rrn && (
        <Link href={`/transactions/${transaction.rrn}`} className="mt-2 block text-sm underline">
          {t("viewJourney")}
        </Link>
      )}
    </div>
  );
}
