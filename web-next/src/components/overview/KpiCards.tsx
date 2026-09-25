import { useTranslations } from "next-intl";
import { useDisplayMode } from "@/shared/state/display-mode";
import type { Overview } from "@/shared/api/overview-client";

// docs/06-user-stories.md MCN-306: p99 under 300ms is the "fast" band on the console.
const FAST_LATENCY_MS = 300;

const CARD_CLASS = "flex flex-col gap-1.5 rounded-card border border-border bg-surface px-5 py-4.5";
const LABEL_CLASS = "text-[13px] font-medium text-muted";
const VALUE_CLASS = "text-[28px] font-bold tracking-tight tabular-nums";
const SUB_CLASS = "text-[13px] text-muted";
const SUB_TECH_CLASS = "font-mono text-xs text-muted";

function peakSample(throughput: Overview["throughput"]): { tps: number; at: string } | null {
  const peak = throughput.reduce<Overview["throughput"][number] | null>(
    (best, sample) => (best === null || sample.tps > best.tps ? sample : best),
    null,
  );
  if (peak === null) return null;
  const at = new Date(peak.at);
  const label = Number.isNaN(at.getTime())
    ? peak.at
    : at.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", hour12: false });
  return { tps: peak.tps, at: label };
}

export function KpiCards({ overview }: { overview: Overview }) {
  const t = useTranslations("overview.kpi");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const deltaPct = overview.transactionsDeltaPct;
  const peak = peakSample(overview.throughput);
  const sub = expert ? SUB_TECH_CLASS : SUB_CLASS;

  return (
    <dl className="grid grid-cols-2 gap-4 xl:grid-cols-4">
      <div className={CARD_CLASS}>
        <dt className={LABEL_CLASS}>{t("transactionsToday")}</dt>
        <dd className={VALUE_CLASS}>{overview.transactionsToday.toLocaleString("vi-VN")}</dd>
        {expert ? (
          peak !== null && <p className={SUB_TECH_CLASS}>{t("transactionsTech", { peak: peak.tps, at: peak.at })}</p>
        ) : (
          deltaPct !== undefined && (
            <p className={deltaPct >= 0 ? "text-[13px] font-medium text-ok" : SUB_CLASS}>
              {t(deltaPct >= 0 ? "transactionsUp" : "transactionsDown", {
                pct: Math.abs(Math.round(deltaPct * 100)),
              })}
            </p>
          )
        )}
      </div>
      <div className={CARD_CLASS}>
        <dt className={LABEL_CLASS}>{t("approvalRate")}</dt>
        <dd className={VALUE_CLASS}>{(overview.approvalRate * 100).toLocaleString("vi-VN", { maximumFractionDigits: 1 })}%</dd>
        <p className={sub}>{t(expert ? "approvalTech" : "approvalSteady")}</p>
      </div>
      <div className={CARD_CLASS}>
        <dt className={LABEL_CLASS}>{t("p99Latency")}</dt>
        <dd className={VALUE_CLASS}>{overview.p99LatencyMs} ms</dd>
        <p className={sub}>
          {expert
            ? t("latencyTech")
            : t(overview.p99LatencyMs < FAST_LATENCY_MS ? "latencyFast" : "latencySlow", {
                threshold: FAST_LATENCY_MS,
              })}
        </p>
      </div>
      <div
        className={CARD_CLASS}
        data-testid="kpi-ledger"
        data-status={overview.ledgerMatches ? "ok" : "warn"}
      >
        <dt className={LABEL_CLASS}>{t("ledgerMatches")}</dt>
        <dd className={VALUE_CLASS}>{overview.ledgerMatches ? t("ledgerOk") : t("ledgerMismatch")}</dd>
        <p className={sub}>
          {expert
            ? t(overview.ledgerMatches ? "ledgerTech" : "ledgerTechMismatch")
            : t(overview.ledgerMatches ? "ledgerNoGap" : "ledgerHasGap")}
        </p>
      </div>
    </dl>
  );
}
