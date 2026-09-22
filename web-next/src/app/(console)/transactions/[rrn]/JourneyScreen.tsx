"use client";

import { useEffect, useRef, useState } from "react";
import { useJourney } from "@/shared/api/journey-client";
import { SummaryHeader } from "@/components/journey/SummaryHeader";
import { StepTimeline } from "@/components/journey/StepTimeline";
import { StepDetail } from "@/components/journey/StepDetail";
import { MoneyPanel } from "@/components/journey/MoneyPanel";
import { PlaybackControls } from "@/components/journey/PlaybackControls";

/** MCN-307 ruling: 2s/step autoplay pace — matches an "explain what happened" pacing. */
const AUTOPLAY_INTERVAL_MS = 2000;

export function JourneyScreen({ rrn }: { rrn: string }) {
  const { data: journey } = useJourney(rrn);
  const [currentStep, setCurrentStep] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const totalSteps = journey?.steps.length ?? 0;

  useEffect(() => {
    if (!isPlaying || totalSteps === 0) return;
    intervalRef.current = setInterval(() => {
      setCurrentStep((step) => {
        const next = step + 1;
        if (next >= totalSteps) {
          setIsPlaying(false);
          return step;
        }
        return next;
      });
    }, AUTOPLAY_INTERVAL_MS);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [isPlaying, totalSteps]);

  function selectStep(index: number) {
    setIsPlaying(false);
    setCurrentStep(Math.max(0, Math.min(index, totalSteps - 1)));
  }

  if (!journey) return null;

  return (
    <div className="space-y-6">
      <SummaryHeader transaction={journey.transaction} />
      <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <PlaybackControls
            currentStep={currentStep}
            totalSteps={totalSteps}
            isPlaying={isPlaying}
            onStepChange={selectStep}
            onTogglePlay={() => setIsPlaying((playing) => !playing)}
            onRestart={() => selectStep(0)}
          />
          <StepTimeline steps={journey.steps} currentStep={currentStep} onSelectStep={selectStep} />
        </div>
        <aside className="space-y-4">
          <StepDetail step={journey.steps[currentStep]} />
          <MoneyPanel money={journey.money} currency={journey.transaction.amount.currency} />
        </aside>
      </div>
    </div>
  );
}
