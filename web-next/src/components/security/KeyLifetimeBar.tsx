"use client";

import { useTranslations } from "next-intl";

const EXPIRY_WARNING_THRESHOLD_DAYS = 30;

export function KeyLifetimeBar({ daysRemaining, lifetimeDays }: { daysRemaining: number; lifetimeDays: number }) {
  const t = useTranslations("security.keys");
  const ratio = lifetimeDays > 0 ? Math.min(Math.max(daysRemaining / lifetimeDays, 0), 1) : 0;
  const isNearExpiry = daysRemaining < EXPIRY_WARNING_THRESHOLD_DAYS;

  return (
    <div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-canvas">
        <div
          className={`key-lifetime-fill h-full w-full rounded-full ${isNearExpiry ? "bg-warn" : "bg-accent"}`}
          style={{ transform: `scaleX(${ratio})` }}
        />
      </div>
      {isNearExpiry && (
        <p role="alert" className="mt-1 text-xs font-medium text-warn">
          {t("expiryWarning", { days: daysRemaining })}
        </p>
      )}
    </div>
  );
}
