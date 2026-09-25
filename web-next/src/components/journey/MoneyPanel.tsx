import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { MoneyRow, Outcome } from "./journey-model";
import "./journey.css";

interface MoneyPanelProps {
  rows: MoneyRow[];
  currency: string;
  currentStep: number;
  outcome: Outcome;
}

/** "Tiền trong tài khoản khách": each row lights up once playback reaches the step that moved it. */
export function MoneyPanel({ rows, currency, currentStep, outcome }: MoneyPanelProps) {
  const t = useTranslations("journey.money");
  return (
    <section aria-labelledby="journey-money-heading" className="flex flex-col gap-1 rounded-card border border-border bg-surface px-[22px] py-5">
      <h2 id="journey-money-heading" className="mb-2 text-[17px] font-semibold">
        {t("heading")}
      </h2>
      {rows.map((row, i) => (
        <div
          key={`${row.kind}-${i}`}
          className="journey-money-row"
          data-kind={row.kind}
          data-sign={row.amount < 0 ? "debit" : "credit"}
          data-reached={row.atIndex <= currentStep}
        >
          <span>{t(row.kind)}</span>
          <span className="journey-money-value">{signed(row, currency)}</span>
        </div>
      ))}
      <p className="mt-2 text-[13px] leading-normal text-muted">{t(`note.${outcome}`)}</p>
    </section>
  );
}

function signed(row: MoneyRow, currency: string): string {
  const text = formatMoney({ amount: Math.abs(row.amount), currency });
  if (row.kind === "debit") return `−${text}`;
  if (row.kind === "credit") return `+${text}`;
  return text;
}
