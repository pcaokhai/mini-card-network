import { useTranslations } from "next-intl";
import type { SettlementDay } from "@/shared/api/settlement-client";
import { formatMoney } from "@/shared/format/money";

/** What the issuer owes the acquirer after netting, from `SettlementDay.netPosition`. */
export function NetPositionCard({ day, expert, dayLabel }: { day: SettlementDay; expert: boolean; dayLabel: string }) {
  const t = useTranslations("settlement.net");
  return (
    <section aria-label={t("label")} className="set-net">
      <p className="set-net__label">{expert ? t("titleExpert", { day: dayLabel }) : t("titleEasy")}</p>
      <p className="set-net__amount">{day.netPosition ? formatMoney(day.netPosition) : t("unknown")}</p>
      <p className="set-net__desc">{t(expert ? "descExpert" : "descEasy")}</p>
    </section>
  );
}
