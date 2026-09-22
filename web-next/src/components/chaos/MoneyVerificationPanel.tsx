import { useTranslations } from "next-intl";
import { useEffect, useRef, useState } from "react";
import { formatMoney } from "@/shared/format/money";
import type { ChaosRun } from "@/shared/api/chaos-client";

type Result = "pending" | "ok" | "discrepancy";

const CURRENCY_VND = "704";

function resultFor(status: ChaosRun["status"]): Result {
  if (status === "PASSED") return "ok";
  if (status === "FAILED") return "discrepancy";
  return "pending";
}

export function MoneyVerificationPanel({ run }: { run: ChaosRun | undefined }) {
  const t = useTranslations("chaos.money");
  const previousStatusRef = useRef<ChaosRun["status"] | undefined>(undefined);
  const [flash, setFlash] = useState(false);

  const result = run ? resultFor(run.status) : "pending";

  useEffect(() => {
    const previous = previousStatusRef.current;
    previousStatusRef.current = run?.status;
    if (run && previous !== run.status && (run.status === "PASSED" || run.status === "FAILED")) {
      setFlash(true);
      const timeout = setTimeout(() => setFlash(false), 600);
      return () => clearTimeout(timeout);
    }
  }, [run, run?.status]);

  return (
    <section
      aria-labelledby="chaos-money-heading"
      data-testid="money-verification"
      data-result={result}
      className={flash ? "money-verification--flash rounded-lg border border-canvas p-4" : "rounded-lg border border-canvas p-4"}
    >
      <h2 id="chaos-money-heading" className="mb-2 text-sm font-semibold">
        {t("heading")}
      </h2>
      {run ? (
        <dl className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <dt className="text-xs text-muted">{t("opening")}</dt>
            <dd className="font-mono">{formatMoney({ amount: run.openingBalanceTotal ?? 0, currency: CURRENCY_VND })}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted">{t("closing")}</dt>
            <dd className="font-mono">{formatMoney({ amount: run.closingBalanceTotal ?? 0, currency: CURRENCY_VND })}</dd>
          </div>
          <div className="col-span-2">
            <dt className="text-xs text-muted">{t("discrepancy")}</dt>
            <dd className="font-mono">{formatMoney({ amount: run.ledgerDiscrepancy ?? 0, currency: CURRENCY_VND })}</dd>
          </div>
          {result === "discrepancy" && (
            <div className="col-span-2 text-sm font-semibold text-bad">{t("discrepancyRun", { runId: run.runId })}</div>
          )}
        </dl>
      ) : (
        <p className="text-sm text-muted">{t("empty")}</p>
      )}
    </section>
  );
}
