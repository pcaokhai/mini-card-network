import { useTranslations } from "next-intl";
import { formatMoney, type Money } from "@/shared/format/money";

export interface Hold {
  holdId: string;
  merchantName: string;
  amount: Money;
  expiresAt: string;
  status: "ACTIVE" | "COMPLETED" | "RELEASED" | "EXPIRED";
}

const STATUS_CLASSES: Record<Hold["status"], string> = {
  ACTIVE: "bg-accent-soft text-accent",
  COMPLETED: "bg-ok-soft text-ok",
  RELEASED: "bg-canvas text-muted",
  EXPIRED: "bg-bad-soft text-bad",
};

function HoldStatusPill({ status }: { status: Hold["status"] }) {
  const t = useTranslations("cards.holds.status");
  return (
    <span
      data-testid="hold-status-pill"
      data-status={status}
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ${STATUS_CLASSES[status]}`}
    >
      <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-current" />
      {t(status)}
    </span>
  );
}

export function HoldsList({ holds }: { holds: Hold[] }) {
  const t = useTranslations("cards.holds");
  if (holds.length === 0) return <p className="text-sm text-muted">{t("empty")}</p>;
  return (
    <table className="w-full text-sm">
      <caption className="mb-1 text-left text-xs text-muted">{t("title")}</caption>
      <tbody>
        {holds.map((hold) => (
          <tr key={hold.holdId}>
            <td className="py-1">{hold.merchantName}</td>
            <td className="py-1 text-right font-mono">{formatMoney(hold.amount)}</td>
            <td className="py-1 text-right text-muted">{new Date(hold.expiresAt).toLocaleDateString()}</td>
            <td className="py-1 text-right">
              <HoldStatusPill status={hold.status} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
