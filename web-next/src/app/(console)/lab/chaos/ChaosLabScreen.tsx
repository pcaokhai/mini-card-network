"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useState } from "react";
import {
  CHAOS_SCENARIO_IDS,
  ChaosRunGoneError,
  chaosRunKey,
  isRunFinished,
  newerRun,
  useChaosRun,
  useChaosScenarios,
  useDisableAllChaosScenarios,
  useSetChaosScenario,
  useStartChaosRun,
  type ChaosRun,
} from "@/shared/api/chaos-client";
import { useSafQueue } from "@/shared/api/network-client";
import { useOverview } from "@/shared/api/overview-client";
import { useWsEvents, type WsEnvelope } from "@/shared/ws/useWsEvents";
import { useDisplayMode } from "@/shared/state/display-mode";
import { ScenarioCard } from "@/components/chaos/ScenarioCard";
import { MoneyVerificationPanel } from "@/components/chaos/MoneyVerificationPanel";
import { ReactionPanel } from "@/components/chaos/ReactionPanel";
import "@/components/chaos/chaos.css";

const RUN_TRANSACTIONS = 100;
const RUN_EVENT_TYPES = ["chaos.run.progress", "chaos.changed"];

export function ChaosLabScreen() {
  const t = useTranslations("chaos");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const queryClient = useQueryClient();

  const scenariosQuery = useChaosScenarios();
  const scenarios = scenariosQuery.data ?? [];
  const setScenario = useSetChaosScenario();
  const disableAll = useDisableAllChaosScenarios();
  const startRun = useStartChaosRun();
  const safDepth = useSafQueue().data?.depth;
  const p99LatencyMs = useOverview().data?.p99LatencyMs;

  const [runId, setRunId] = useState<string | null>(null);
  const runQuery = useChaosRun(runId);
  // A run the gateway forgot (restart) is treated as no run: the panel resets and a new run can
  // start; the query has stopped polling it (CHA-G7).
  const run = runQuery.error instanceof ChaosRunGoneError ? undefined : runQuery.data;
  const running = run !== undefined && !isRunFinished(run);

  useWsEvents(RUN_EVENT_TYPES, (event: WsEnvelope) => {
    if (event.type === "chaos.changed") {
      void queryClient.invalidateQueries({ queryKey: ["chaos", "scenarios"] });
      return;
    }
    const data = event.data as ChaosRun;
    if (data.runId === runId) queryClient.setQueryData<ChaosRun>(chaosRunKey(data.runId), (cached) => newerRun(cached, data));
  });

  const active = scenarios.filter((s) => s.enabled).map((s) => s.id);
  const toggleFailed = setScenario.isError || disableAll.isError;

  const onRun = () =>
    startRun.mutate(RUN_TRANSACTIONS, {
      onSuccess: (started) => {
        queryClient.setQueryData(chaosRunKey(started.runId), started);
        setRunId(started.runId);
      },
    });

  return (
    <div className="chaos-root">
      <div className="chaos-title-row">
        <div>
          <h1 className="m-0 text-[30px] font-bold tracking-[-0.01em]">{t("heading")}</h1>
          <p className="mt-1.5 mb-0 text-[15px] text-muted">{t("subtitle")}</p>
        </div>
        <div className="chaos-actions">
          {scenariosQuery.isSuccess && (
            <span className="chaos-pill" data-testid="chaos-active-count" data-tone={active.length === 0 ? "ok" : "warn"} aria-live="polite">
              {active.length === 0 ? t("count.calm") : t("count.active", { count: active.length })}
            </span>
          )}
          <button type="button" className="chaos-btn" onClick={() => disableAll.mutate(active)}>
            {t("resetAll")}
          </button>
          <button type="button" className="chaos-btn chaos-btn--primary" disabled={running || startRun.isPending} onClick={onRun}>
            {t("runBatch", { count: RUN_TRANSACTIONS })}
          </button>
        </div>
      </div>

      {scenariosQuery.isError && (
        <p role="alert" className="chaos-alert">
          {t("error.scenarios")}
        </p>
      )}
      {(toggleFailed || startRun.isError) && (
        <p role="alert" className="chaos-alert">
          {startRun.isError ? t("error.run") : t("error.toggle")}
        </p>
      )}

      <div className="chaos-columns">
        <div className="chaos-cards">
          {scenariosQuery.isSuccess && CHAOS_SCENARIO_IDS.map((id) => (
            <ScenarioCard
              key={id}
              id={id}
              enabled={active.includes(id)}
              available={scenarios.find((s) => s.id === id)?.available}
              expert={expert}
              busy={setScenario.isPending && setScenario.variables.scenarioId === id}
              onToggle={(scenarioId, enabled) => setScenario.mutate({ scenarioId, enabled })}
            />
          ))}
        </div>
        <div className="chaos-side">
          <MoneyVerificationPanel run={run} expert={expert} />
          <ReactionPanel activeScenarios={active} expert={expert} safDepth={safDepth} p99LatencyMs={p99LatencyMs} />
        </div>
      </div>
    </div>
  );
}
