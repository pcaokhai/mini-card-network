"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { useCustomerLedger, useJourney, type Journey } from "@/shared/api/journey-client";
import { CountdownRing } from "./CountdownRing";
import { MoneyPanel } from "./MoneyPanel";
import { PlaybackControls } from "./PlaybackControls";
import { StepDetail } from "./StepDetail";
import { StepTimeline } from "./StepTimeline";
import { SummaryHeader } from "./SummaryHeader";
import { customerBalances, moneyRows, outcomeOf } from "./journey-model";
import { useJourneyCopy } from "./useJourneyCopy";

/** The canvas's autoplay pace (Journey.dc.html `play`); a 30 s timeout plays as one accelerated step. */
export const AUTOPLAY_INTERVAL_MS = 950;

export function JourneyView({ rrn }: { rrn: string }) {
  const t = useTranslations("journey");
  const { data: journey, isError } = useJourney(rrn);
  if (isError) return <Notice>{t("notFound", { rrn })}</Notice>;
  if (!journey) return null;
  // Keyed on the RRN so switching transactions resets playback to the final step.
  return <LoadedJourney key={rrn} journey={journey} />;
}

function LoadedJourney({ journey }: { journey: Journey }) {
  const copy = useJourneyCopy(journey);
  const total = journey.steps.length;
  const [current, setCurrent] = useState(total - 1);
  const [playing, setPlaying] = useState(false);
  const ledger = useCustomerLedger(journey.transaction.maskedPan, journey.transaction.rrn);
  const outcome = outcomeOf(journey.transaction.status, journey.steps);

  const rows = useMemo(() => {
    const balances = ledger.data ? customerBalances(ledger.data.newestFirst, ledger.data.currentBalance, journey.transaction.rrn) : null;
    return moneyRows(journey, balances);
  }, [journey, ledger.data]);

  useEffect(() => {
    if (!playing) return;
    const timer = setTimeout(() => {
      if (current >= total - 1) setPlaying(false);
      else setCurrent(current + 1);
    }, AUTOPLAY_INTERVAL_MS);
    return () => clearTimeout(timer);
  }, [playing, current, total]);

  function select(index: number) {
    setPlaying(false);
    setCurrent(Math.max(0, Math.min(index, total - 1)));
  }

  if (total === 0) return null;
  const step = journey.steps[current];
  return (
    <div className="flex flex-col gap-5">
      <SummaryHeader outcome={outcome} content={copy.summary(outcome)} />
      <div className="grid items-start gap-5 min-[1100px]:grid-cols-[minmax(0,1fr)_430px]">
        <StepTimeline
          steps={journey.steps}
          copy={copy.step}
          locale={copy.locale}
          currentStep={current}
          onSelectStep={select}
          controls={
            <PlaybackControls
              currentStep={current}
              totalSteps={total}
              isPlaying={playing}
              onStepChange={select}
              onPlay={() => {
                setCurrent(0);
                setPlaying(true);
              }}
              onRestart={() => select(0)}
            />
          }
        />
        <div className="flex flex-col gap-[18px]">
          <StepDetail
            key={step.seq}
            step={step}
            copy={copy.step(step, current)}
            expert={copy.expert}
            locale={copy.locale}
            extra={step.code === "NO_RESPONSE" && <CountdownRing durationMs={AUTOPLAY_INTERVAL_MS} isPlaying={playing} />}
          />
          <MoneyPanel rows={rows} currency={journey.transaction.amount.currency} currentStep={current} outcome={outcome} />
        </div>
      </div>
    </div>
  );
}

export function Notice({ children }: { children: React.ReactNode }) {
  return <div className="rounded-card border border-border bg-surface px-8 py-7 text-[15px] text-muted">{children}</div>;
}
