import type { ChaosScenario, ChaosScenarioId } from "@/shared/api/chaos-client";

export function ScenarioCard({
  scenario,
  onToggle,
}: {
  scenario: ChaosScenario;
  onToggle: (id: ChaosScenarioId, enabled: boolean) => void;
}) {
  return (
    <div
      data-testid={`scenario-card-${scenario.id}`}
      data-enabled={scenario.enabled}
      className={
        scenario.enabled
          ? "rounded-lg border border-accent bg-accent-soft p-4"
          : "rounded-card border border-border bg-surface p-4"
      }
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-semibold">{scenario.easyText}</span>
        <button
          type="button"
          role="switch"
          aria-checked={scenario.enabled}
          aria-label={scenario.easyText}
          onClick={() => onToggle(scenario.id, !scenario.enabled)}
          className={
            scenario.enabled
              ? "h-6 w-11 rounded-full bg-accent px-0.5 transition-colors"
              : "h-6 w-11 rounded-full bg-[#D9D6CC] px-0.5 transition-colors"
          }
        >
          <span
            className={
              scenario.enabled
                ? "block h-5 w-5 translate-x-5 rounded-full bg-surface transition-transform"
                : "block h-5 w-5 translate-x-0 rounded-full bg-surface transition-transform"
            }
          />
        </button>
      </div>
      <p className="mt-1 text-xs text-muted">{scenario.technicalText}</p>
    </div>
  );
}
