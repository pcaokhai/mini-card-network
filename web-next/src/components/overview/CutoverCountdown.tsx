import { useTranslations } from "next-intl";
import Link from "next/link";
import { useEffect, useState } from "react";

// docs/03-iso8583-interface-spec.md §7.6: default cutover time is 23:59:59 local.
const CUTOVER_HOUR = 23;
const CUTOVER_MINUTE = 59;
const CUTOVER_SECOND = 59;

const MS_PER_MINUTE = 60_000;
const MINUTES_PER_HOUR = 60;

function msUntilNextCutover(now: Date): number {
  const cutover = new Date(now);
  cutover.setHours(CUTOVER_HOUR, CUTOVER_MINUTE, CUTOVER_SECOND, 0);
  if (cutover.getTime() <= now.getTime()) cutover.setDate(cutover.getDate() + 1);
  return cutover.getTime() - now.getTime();
}

function businessDayLabel(remainingMs: number): string {
  const cutover = new Date(Date.now() + remainingMs);
  const day = String(cutover.getDate()).padStart(2, "0");
  const month = String(cutover.getMonth() + 1).padStart(2, "0");
  return `${day}/${month}`;
}

export function CutoverCountdown() {
  const t = useTranslations("overview.cutover");
  const [remainingMs, setRemainingMs] = useState(() => msUntilNextCutover(new Date()));

  useEffect(() => {
    const id = setInterval(() => setRemainingMs(msUntilNextCutover(new Date())), 1000);
    return () => clearInterval(id);
  }, []);

  const totalMinutes = Math.max(0, Math.floor(remainingMs / MS_PER_MINUTE));
  const hours = Math.floor(totalMinutes / MINUTES_PER_HOUR);
  const minutes = totalMinutes % MINUTES_PER_HOUR;

  return (
    <section aria-label={t("heading")} className="flex flex-col gap-2.5 rounded-card bg-ink px-5.5 py-5 text-white">
      <p className="text-[13px] text-[#C9CBD2]">{t("heading")}</p>
      <p className="text-[22px] font-bold" data-testid="cutover-remaining">
        {t("remaining", { hours, minutes })}
      </p>
      <p className="text-[13px] leading-relaxed text-[#C9CBD2]" suppressHydrationWarning>
        {t("body", { date: businessDayLabel(remainingMs) })}
      </p>
      <Link
        href="/settlement"
        className="mt-1 inline-flex h-10 items-center self-start rounded-[10px] bg-surface px-4 text-sm font-semibold text-ink"
      >
        {t("openSettlement")}
      </Link>
    </section>
  );
}
