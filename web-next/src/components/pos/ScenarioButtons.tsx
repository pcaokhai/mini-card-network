"use client";

export type Scenario = "approved" | "insufficient-funds" | "blocked-card" | "timeout" | "duplicate";

/**
 * One-click presets that set card/amount/entry-mode to values expected to trigger each
 * outcome. timeout/duplicate are MSW-mock-only fakes this story simulates - real dedupe and
 * timeout mechanics land with MCN-401/MCN-303.
 */
export const SCENARIO_PRESETS: Record<Scenario, { cardToken: string; amount: number; label: string }> = {
  approved: { cardToken: "tok_normal", amount: 10000, label: "Approved" },
  "insufficient-funds": { cardToken: "tok_low", amount: 500000, label: "Declined - insufficient funds" },
  "blocked-card": { cardToken: "tok_blocked", amount: 10000, label: "Declined - blocked card" },
  timeout: { cardToken: "tok_normal", amount: 10000, label: "Network timeout" },
  duplicate: { cardToken: "tok_normal", amount: 10000, label: "Duplicate tap" },
};

interface ScenarioButtonsProps {
  onPick: (scenario: Scenario) => void;
}

export function ScenarioButtons({ onPick }: ScenarioButtonsProps) {
  return (
    <div role="group" aria-label="Scenarios" className="flex flex-wrap gap-2">
      {(Object.keys(SCENARIO_PRESETS) as Scenario[]).map((scenario) => (
        <button
          key={scenario}
          type="button"
          onClick={() => onPick(scenario)}
          className="rounded-lg border border-border bg-surface px-3 py-1.5 text-xs font-medium text-ink hover:bg-canvas"
        >
          {SCENARIO_PRESETS[scenario].label}
        </button>
      ))}
    </div>
  );
}
