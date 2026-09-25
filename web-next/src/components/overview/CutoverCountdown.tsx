import { useTranslations } from "next-intl";
import { useEffect, useState } from "react";

// docs/03-iso8583-interface-spec.md §7.6: default cutover time is 23:59:59 local.
const CUTOVER_HOUR = 23;
const CUTOVER_MINUTE = 59;
const CUTOVER_SECOND = 59;

function msUntilNextCutover(now: Date): number {
  const cutover = new Date(now);
  cutover.setHours(CUTOVER_HOUR, CUTOVER_MINUTE, CUTOVER_SECOND, 0);
  if (cutover.getTime() <= now.getTime()) cutover.setDate(cutover.getDate() + 1);
  return cutover.getTime() - now.getTime();
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const hours = String(Math.floor(totalSeconds / 3600)).padStart(2, "0");
  const minutes = String(Math.floor((totalSeconds % 3600) / 60)).padStart(2, "0");
  const seconds = String(totalSeconds % 60).padStart(2, "0");
  return `${hours}:${minutes}:${seconds}`;
}

export function CutoverCountdown() {
  const t = useTranslations("overview.cutover");
  const [remainingMs, setRemainingMs] = useState(() => msUntilNextCutover(new Date()));

  useEffect(() => {
    const id = setInterval(() => setRemainingMs(msUntilNextCutover(new Date())), 1000);
    return () => clearInterval(id);
  }, []);

  return (
    <div className="rounded-card bg-ink p-4 text-white">
      <h2 className="mb-1 text-lg font-semibold">{t("heading")}</h2>
      <p>
        {t("label")} <span data-testid="cutover-remaining">{formatDuration(remainingMs)}</span>
      </p>
    </div>
  );
}
