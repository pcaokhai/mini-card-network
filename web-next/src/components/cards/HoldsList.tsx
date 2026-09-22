import { useTranslations } from "next-intl";
import { formatMoney, type Money } from "@/shared/format/money";

export interface Hold {
  holdId: string;
  merchantName: string;
  amount: Money;
  expiresAt: string;
  status: "ACTIVE" | "COMPLETED" | "RELEASED" | "EXPIRED";
}

export function HoldsList({ holds }: { holds: Hold[] }) {
  const t = useTranslations("cards.holds");
  const active = holds.filter((h) => h.status === "ACTIVE");
  if (active.length === 0) return <p className="text-sm text-muted">{t("empty")}</p>;
  return (
    <table className="w-full text-sm">
      <caption className="mb-1 text-left text-xs text-muted">{t("title")}</caption>
      <tbody>
        {active.map((hold) => (
          <tr key={hold.holdId}>
            <td className="py-1">{hold.merchantName}</td>
            <td className="py-1 text-right font-mono">{formatMoney(hold.amount)}</td>
            <td className="py-1 text-right text-muted">{new Date(hold.expiresAt).toLocaleDateString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
