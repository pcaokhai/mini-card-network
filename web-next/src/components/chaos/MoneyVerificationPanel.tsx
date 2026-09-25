import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { ChaosRun } from "@/shared/api/chaos-client";
import { ledgerRows, runVerdict, type LedgerValue, type RunVerdict } from "./chaos-model";

const CURRENCY_VND = "704";
const BADGE_TONE: Record<RunVerdict, string> = { none: "info", pending: "info", ok: "ok", discrepancy: "bad" };

/** "Kiểm chứng tiền": the latest run's ledger invariant, with the zero difference flashing green on completion. */
export function MoneyVerificationPanel({ run, expert }: { run: ChaosRun | undefined; expert: boolean }) {
  const t = useTranslations("chaos.money");
  const verdict = runVerdict(run);
  const mode = expert ? "expert" : "easy";

  const valueText = (value: LedgerValue) => {
    switch (value.kind) {
      case "none":
        return "—";
      case "pending":
        return "…";
      case "count":
        return t("count", { count: value.amount });
      case "debit":
        return `−${formatMoney({ amount: value.amount, currency: CURRENCY_VND })}`;
      case "money":
        return formatMoney({ amount: value.amount, currency: CURRENCY_VND });
    }
  };

  return (
    <section aria-labelledby="chaos-money-heading" data-testid="money-verification" data-result={verdict} className="chaos-panel chaos-money">
      <div className="chaos-money__head">
        <h2 id="chaos-money-heading" className="chaos-panel__heading">
          {t("heading")}
        </h2>
        <span className="chaos-pill" data-tone={BADGE_TONE[verdict]} role="status">
          {verdict === "ok" && (
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M5 12l5 5 9-10" />
            </svg>
          )}
          {t(`badge.${verdict}`)}
        </span>
      </div>
      <p className="chaos-panel__note">{t("note")}</p>
      <dl className="chaos-ledger">
        {ledgerRows(run).map((row) => (
          <div key={row.key} className="chaos-ledger__row">
            <dt>
              {row.key === "approved" && row.count === undefined
                ? t(`${mode}.approvedNone`)
                : t(`${mode}.${row.key}`, { count: new Intl.NumberFormat("vi-VN").format(row.count ?? 0) })}
            </dt>
            <dd
              // Keyed by run so the green flash replays for every finished run (Ruling R6).
              key={run?.runId}
              className="chaos-ledger__value"
              data-testid={`ledger-${row.key}-value`}
              data-tone={row.tone}
              data-flash={row.tone === "ok" ? true : undefined}
            >
              {valueText(row.value)}
            </dd>
          </div>
        ))}
      </dl>
      {verdict === "discrepancy" && run && <p className="chaos-ledger__alert">{t("discrepancyRun", { runId: run.runId })}</p>}
    </section>
  );
}
