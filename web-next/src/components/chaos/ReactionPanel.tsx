import { useTranslations } from "next-intl";
import type { ChaosScenarioId } from "@/shared/api/chaos-client";

export function ReactionPanel({
  activeScenarios,
  expertMode,
}: {
  activeScenarios: ChaosScenarioId[];
  expertMode: boolean;
}) {
  const t = useTranslations("chaos");
  return (
    <section aria-labelledby="chaos-reaction-heading" className="rounded-lg border border-canvas p-4">
      <h2 id="chaos-reaction-heading" className="mb-2 text-sm font-semibold">
        {t("reaction.heading")}
      </h2>
      {activeScenarios.length === 0 ? (
        <p className="text-sm text-muted">{t("reaction.empty")}</p>
      ) : (
        <ul className="space-y-2 text-sm">
          {activeScenarios.map((id) => (
            <li key={id}>
              <span className="font-semibold">{t(`scenarios.${id}.easyText`)}</span>
              {" — "}
              <span>{t(`scenarios.${id}.reaction`)}</span>
              {expertMode && (
                <span data-expert-only className="ml-2 font-mono text-xs text-muted">
                  {t(`scenarios.${id}.technicalText`)}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
