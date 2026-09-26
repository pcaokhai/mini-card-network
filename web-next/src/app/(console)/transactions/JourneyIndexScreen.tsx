"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { JOURNEY_VIEWS, JourneyHeader, VIEW_REVERSAL_REASON, VIEW_STATUS, type JourneyView as View } from "@/components/journey/JourneyHeader";
import { JourneyView, Notice } from "@/components/journey/JourneyView";
import { useLatestTransaction } from "@/shared/api/journey-client";

function viewFrom(param: string | null): View {
  return JOURNEY_VIEWS.find((v) => v === param) ?? "approved";
}

/**
 * /transactions: the canvas's scenario tabs, each opening the newest real transaction with that
 * outcome. The tab lives in the URL (?view=) so it can be shared and survives a reload.
 */
export function JourneyIndexScreen() {
  const t = useTranslations("journey");
  const router = useRouter();
  const view = viewFrom(useSearchParams().get("view"));
  const latest = useLatestTransaction(VIEW_STATUS[view], view === "reversed" ? VIEW_REVERSAL_REASON.reversed : undefined);

  return (
    <div className="flex flex-col gap-5">
      <JourneyHeader active={view} onPick={(next) => router.replace(`/transactions?view=${next}`)} />
      {latest.isError && <Notice>{t("loadError")}</Notice>}
      {latest.data === null && (
        <Notice>
          <div className="text-base font-semibold text-ink">{t("empty.title")}</div>
          <p className="mt-1">{t("empty.body")}</p>
          <Link href="/pos" className="mt-3 inline-block font-semibold text-accent">
            {t("empty.cta")}
          </Link>
        </Notice>
      )}
      {latest.data && <JourneyView rrn={latest.data.rrn} />}
    </div>
  );
}
