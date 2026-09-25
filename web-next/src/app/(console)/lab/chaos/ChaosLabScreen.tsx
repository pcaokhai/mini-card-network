"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";
import {
  useChaosRun,
  useChaosScenarios,
  useSetChaosScenario,
  useStartChaosRun,
  type ChaosRun,
  type ChaosScenarioId,
} from "@/shared/api/chaos-client";
import { useWsEvents, type WsEnvelope } from "@/shared/ws/useWsEvents";
import { useDisplayMode } from "@/shared/state/display-mode";
import { ScenarioCard } from "@/components/chaos/ScenarioCard";
import { MoneyVerificationPanel } from "@/components/chaos/MoneyVerificationPanel";
import { ReactionPanel } from "@/components/chaos/ReactionPanel";
import "@/components/chaos/chaos.css";

const DEFAULT_TRANSACTIONS = 50;
const RUN_EVENT_TYPES = ["chaos.run.progress", "chaos.changed"];

export function ChaosLabScreen() {
  const t = useTranslations("chaos");
  const expertMode = useDisplayMode((s) => s.mode === "expert");

  const scenariosQuery = useChaosScenarios();
  const setScenario = useSetChaosScenario();
  const startRun = useStartChaosRun();

  const [transactions, setTransactions] = useState(DEFAULT_TRANSACTIONS);
  const [runId, setRunId] = useState<string | null>(null);
  const [run, setRun] = useState<ChaosRun | undefined>(undefined);

  const runQuery = useChaosRun(runId);
  const polledRun = runQuery.data;
  const activeRun = run ?? polledRun;

  useWsEvents(RUN_EVENT_TYPES, (event: WsEnvelope) => {
    if (event.type !== "chaos.run.progress") return;
    const data = event.data as ChaosRun;
    if (data.runId !== runId) return;
    setRun(data);
  });

  const scenarios = scenariosQuery.data ?? [];
  const activeScenarios: ChaosScenarioId[] = scenarios.filter((s) => s.enabled).map((s) => s.id);

  return (
    <section aria-labelledby="chaos-heading" className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 id="chaos-heading" className="text-2xl font-bold">
          {t("heading")}
        </h1>
        <span data-testid="chaos-active-count" className="text-sm text-muted">
          {t("activeCount", { count: activeScenarios.length })}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        {scenarios.map((scenario) => (
          <ScenarioCard
            key={scenario.id}
            scenario={scenario}
            onToggle={(scenarioId, enabled) => setScenario.mutate({ scenarioId, enabled })}
          />
        ))}
      </div>

      <div className="flex items-end gap-3 rounded-card border border-border bg-surface p-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("runControl.label")}
          <input
            type="number"
            min={1}
            max={10000}
            value={transactions}
            onChange={(e) => setTransactions(Number(e.target.value))}
            className="w-32 rounded-md border border-border bg-surface px-2 py-1"
          />
        </label>
        <button
          type="button"
          onClick={() =>
            startRun.mutate(transactions, {
              onSuccess: (data) => {
                setRun(data);
                setRunId(data.runId);
              },
            })
          }
          className="h-9 rounded-md bg-accent px-4 text-sm font-semibold text-surface"
        >
          {t("runControl.button")}
        </button>
      </div>

      <MoneyVerificationPanel run={activeRun} />
      <ReactionPanel activeScenarios={activeScenarios} expertMode={expertMode} />
    </section>
  );
}
