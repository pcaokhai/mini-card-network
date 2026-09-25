"use client";

import { useTranslations } from "next-intl";
import { SCENARIOS, type ScenarioId } from "./pos-model";

export type Scenario = (typeof SCENARIOS)[number];

interface ScenarioButtonsProps {
  selected: ScenarioId | null;
  onPick: (scenario: Scenario) => void;
}

export function ScenarioButtons({ selected, onPick }: ScenarioButtonsProps) {
  const t = useTranslations("pos.reading");
  return (
    <div className="pos-pills">
      {SCENARIOS.map((scenario) => (
        <button
          key={scenario.id}
          type="button"
          aria-pressed={selected === scenario.id}
          onClick={() => onPick(scenario)}
          className="pos-pill"
        >
          {t(`scenario.${scenario.id}`)}
        </button>
      ))}
    </div>
  );
}
