"use client";

import { useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { BreaksPanel } from "@/components/settlement/BreaksPanel";
import { ClearingFileCard } from "@/components/settlement/ClearingFileCard";
import { NetPositionCard } from "@/components/settlement/NetPositionCard";
import { StageSteps } from "@/components/settlement/StageSteps";
import { TotalsPanel } from "@/components/settlement/TotalsPanel";
import {
  dayLabels,
  hasReached,
  localBusinessDate,
  nextAction,
  resolutionFor,
  type NextAction,
} from "@/components/settlement/settlement-model";
import {
  type ReconBreak,
  type SettlementDay,
  useBreaks,
  useCutover,
  useGenerateClearingFile,
  useReconciliation,
  useResolveBreak,
  useSettlementDay,
} from "@/shared/api/settlement-client";
import { useDisplayMode } from "@/shared/state/display-mode";
// globals.css loads JetBrains Mono 400 only; the canvas sets the clearing file name in 500.
import "@fontsource/jetbrains-mono/500.css";
import "@/components/settlement/settlement.css";

type Failure = { text: string; key: number };

export function SettlementScreen({ businessDate = localBusinessDate() }: { businessDate?: string }) {
  const t = useTranslations("settlement");
  const day = useSettlementDay(businessDate);

  return (
    <section aria-labelledby="settlement-heading" className="set-root flex flex-col gap-5">
      {day.data ? (
        <SettlementDayView day={day.data} businessDate={businessDate} />
      ) : (
        <>
          <Heading />
          {day.error ? <Unavailable detail={day.error.message} /> : <p className="text-muted">{t("loading")}</p>}
        </>
      )}
    </section>
  );
}

function Heading({ children }: { children?: ReactNode }) {
  const t = useTranslations("settlement");
  return (
    <div className="flex items-end justify-between gap-4">
      <div>
        <h1 id="settlement-heading" className="text-[30px] font-bold tracking-[-0.01em]">
          {t("title")}
        </h1>
        <p className="mt-1.5 text-[15px] text-muted">{t("subtitle")}</p>
      </div>
      {children}
    </div>
  );
}

/** MCN-705-AC1/AC2: one container owns every settlement action, so a refusal shows under the steps. */
function SettlementDayView({ day, businessDate }: { day: SettlementDay; businessDate: string }) {
  const t = useTranslations("settlement");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const breaks = useBreaks(businessDate, hasReached(day.stage, "RECONCILED"));
  const cutover = useCutover(businessDate);
  const reconcile = useReconciliation(businessDate);
  const generate = useGenerateClearingFile(businessDate);
  const resolve = useResolveBreak();
  const [failure, setFailure] = useState<Failure | null>(null);
  const [resolvingId, setResolvingId] = useState<string | null>(null);
  const dates = dayLabels(businessDate);
  const action = nextAction(day.stage);
  const busy = action === "waitTotals" || cutover.isPending || reconcile.isPending || generate.isPending;

  // A new key per refusal remounts the alert, so every refused attempt shakes again (Ruling R9).
  const onError = (error: Error) => setFailure({ text: error.message, key: Date.now() });
  const run: Record<NextAction, () => void> = {
    cutover: () => cutover.mutate(undefined, { onError }),
    waitTotals: () => {},
    reconcile: () => reconcile.mutate(undefined, { onError }),
    generate: () => generate.mutate(undefined, { onError }),
  };

  function runNext(next: NextAction) {
    setFailure(null);
    run[next]();
  }

  function onResolve(item: ReconBreak) {
    setFailure(null);
    setResolvingId(item.breakId);
    const note = t(`breaks.action.${item.breakType}`);
    resolve.mutate(
      { breakId: item.breakId, resolution: resolutionFor(item.breakType), note },
      { onError, onSettled: () => setResolvingId(null) },
    );
  }

  return (
    <>
      <Heading>
        {action && (
          <button type="button" className="set-primary" disabled={busy} aria-busy={busy} onClick={() => runNext(action)}>
            {t(`next.${action}`)}
          </button>
        )}
      </Heading>
      <StageSteps stage={day.stage} expert={expert} dates={dates} error={failure} />
      <div className="set-columns">
        <div className="set-stack">
          <TotalsPanel day={day} expert={expert} />
          <BreaksPanel day={day} breaks={breaks.data ?? []} expert={expert} onResolve={onResolve} resolvingId={resolvingId} />
        </div>
        <div className="set-stack">
          <NetPositionCard day={day} expert={expert} dayLabel={dates.day} />
          <ClearingFileCard file={day.clearingFile} expert={expert} />
        </div>
      </div>
    </>
  );
}

/** Ruling R12: the real stack has no settlement service before R7; say so instead of failing. */
function Unavailable({ detail }: { detail: string }) {
  const t = useTranslations("settlement.unavailable");
  return (
    <section aria-labelledby="settlement-unavailable" className="set-card set-unavailable">
      <h2 id="settlement-unavailable" className="set-card__title">
        {t("title")}
      </h2>
      <p className="set-empty">{t("body")}</p>
      <p className="set-unavailable__detail">{t("detail", { detail })}</p>
    </section>
  );
}
