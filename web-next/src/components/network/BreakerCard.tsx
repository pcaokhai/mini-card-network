import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { SwitchStatus } from "@/shared/api/network-client";

const CIRCUITS: SwitchStatus["circuit"][] = ["CLOSED", "OPEN", "HALF_OPEN"];

/** MCN-804-AC1: circuit breaker pills and STIP counters; neutral when the switch reports nothing. */
export function BreakerCard({ switchStatus, expert }: { switchStatus: SwitchStatus | undefined; expert: boolean }) {
  const t = useTranslations("network.breaker");
  const mode = expert ? "tech" : "easy";
  const circuit = switchStatus?.circuit;

  return (
    <section aria-label={t("ariaLabel")} className="net-card gap-3">
      <h2>{t(expert ? "titleTech" : "titleEasy")}</h2>
      <div className="flex gap-1.5">
        {CIRCUITS.map((state) => (
          <span key={state} className="net-pill" data-current={state === circuit} data-tone={state === "CLOSED" ? "ok" : "warn"}>
            {expert ? state : t(`state.${state}`)}
          </span>
        ))}
      </div>
      <p className="net-desc">{t(`desc.${circuit ?? "unavailable"}.${mode}`)}</p>
      <div className="grid grid-cols-2 gap-2.5">
        <div className="net-tile">
          <div className="net-tile__label">{t(expert ? "limitTech" : "limitEasy")}</div>
          <div className="net-tile__value">{switchStatus ? formatMoney(switchStatus.stipLimit) : "—"}</div>
        </div>
        <div className="net-tile">
          <div className="net-tile__label">{t(expert ? "countTech" : "countEasy")}</div>
          <div className="net-tile__value">{switchStatus ? switchStatus.stipApprovedCount.toLocaleString("vi-VN") : "—"}</div>
        </div>
      </div>
    </section>
  );
}
