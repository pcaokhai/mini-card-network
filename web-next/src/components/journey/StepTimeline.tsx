import type { components } from "@/shared/api/generated/schema";
import "./journey.css";

type JourneyStep = components["schemas"]["JourneyStep"];

interface StepTimelineProps {
  steps: JourneyStep[];
  currentStep: number;
  onSelectStep: (index: number) => void;
}

export function StepTimeline({ steps, currentStep, onSelectStep }: StepTimelineProps) {
  return (
    <ol className="step-timeline flex flex-col gap-3">
      {steps.map((step, index) => {
        const state = index < currentStep ? "past" : index === currentStep ? "current" : "future";
        return (
          <li
            key={step.seq}
            data-state={state}
            onClick={() => onSelectStep(index)}
            className="cursor-pointer rounded-card border border-border bg-surface p-3"
          >
            <div className="text-xs font-mono text-muted">{step.actor}</div>
            <div className="font-semibold">{step.title}</div>
          </li>
        );
      })}
    </ol>
  );
}
