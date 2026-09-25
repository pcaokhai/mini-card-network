import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";

export function KpiCards({ overview }: { overview: Overview }) {
  const t = useTranslations("overview.kpi");
  return (
    <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      <div className="rounded-card border border-border bg-surface p-4">
        <dt className="text-xs font-medium text-muted">{t("transactionsToday")}</dt>
        <dd className="text-2xl font-bold">{overview.transactionsToday}</dd>
      </div>
      <div className="rounded-card border border-border bg-surface p-4">
        <dt className="text-xs font-medium text-muted">{t("approvalRate")}</dt>
        <dd className="text-2xl font-bold">{Math.round(overview.approvalRate * 100)}%</dd>
      </div>
      <div className="rounded-card border border-border bg-surface p-4">
        <dt className="text-xs font-medium text-muted">{t("p99Latency")}</dt>
        <dd className="text-2xl font-bold">{overview.p99LatencyMs} ms</dd>
      </div>
      <div
        className="rounded-card border border-border bg-surface p-4"
        data-testid="kpi-ledger"
        data-status={overview.ledgerMatches ? "ok" : "warn"}
      >
        <dt className="text-xs font-medium text-muted">{t("ledgerMatches")}</dt>
        <dd className="text-2xl font-bold">{overview.ledgerMatches ? t("ledgerOk") : t("ledgerMismatch")}</dd>
      </div>
    </dl>
  );
}
