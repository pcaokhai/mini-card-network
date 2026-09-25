"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { useChaosScenarios, useSetChaosScenario } from "@/shared/api/chaos-client";
import {
  useLinkAction,
  useLinks,
  useNetworkEvents,
  useNetworkLiveUpdates,
  useRefreshNetwork,
  useSafQueue,
  useSwitchStatus,
  useTerminals,
} from "@/shared/api/network-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import { BreakerCard } from "@/components/network/BreakerCard";
import { EventLog } from "@/components/network/EventLog";
import { LinksPanel } from "@/components/network/LinksPanel";
import { NetworkTopology } from "@/components/network/NetworkTopology";
import { SafCard } from "@/components/network/SafCard";
import "@/components/network/network.css";

const CLOCK_TICK_MS = 1000;

/** Echo ages count up between polls. */
function useClock(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), CLOCK_TICK_MS);
    return () => clearInterval(id);
  }, []);
  return now;
}

/** MCN-804-AC2: the button drives the ISSUER_DOWN chaos scenario; the screen reacts through the network queries. */
function IssuerDownToggle() {
  const t = useTranslations("network.toggle");
  const scenarios = useChaosScenarios();
  const setScenario = useSetChaosScenario();
  const refreshNetwork = useRefreshNetwork();
  const down = scenarios.data?.find((s) => s.id === "ISSUER_DOWN")?.enabled ?? false;

  return (
    <button
      type="button"
      className="net-toggle"
      data-down={down}
      disabled={scenarios.data === undefined || setScenario.isPending}
      onClick={() => setScenario.mutate({ scenarioId: "ISSUER_DOWN", enabled: !down }, { onSettled: refreshNetwork })}
    >
      {t(down ? "restore" : "down")}
    </button>
  );
}

export function NetworkScreen() {
  const t = useTranslations("network");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const now = useClock();
  const links = useLinks();
  const events = useNetworkEvents();
  const saf = useSafQueue();
  const switchStatus = useSwitchStatus();
  const terminals = useTerminals();
  const linkAction = useLinkAction();
  useNetworkLiveUpdates();

  return (
    <section aria-labelledby="network-heading" className="net-root flex flex-col gap-5">
      <div className="flex items-end justify-between gap-4">
        <div className="min-w-0">
          <h1 id="network-heading" className="text-[30px] font-bold tracking-[-0.01em]">
            {t("title")}
          </h1>
          <p className="mt-1.5 text-[15px] text-muted">{t("subtitle")}</p>
        </div>
        <IssuerDownToggle />
      </div>

      <NetworkTopology
        links={links.data ?? []}
        switchStatus={switchStatus.data}
        terminalCount={terminals.data?.length}
        expert={expert}
      />

      <div className="net-columns">
        <div className="flex min-w-0 flex-col gap-5">
          <LinksPanel
            links={links.data ?? []}
            expert={expert}
            now={now}
            pendingLinkId={linkAction.isPending ? (linkAction.variables?.linkId ?? null) : null}
            onEcho={(linkId) => linkAction.mutate({ linkId, action: "echo" })}
          />
          <EventLog events={events.data ?? []} expert={expert} now={now} />
        </div>
        <div className="flex flex-col gap-5">
          <BreakerCard switchStatus={switchStatus.data} expert={expert} />
          <SafCard saf={saf.data} expert={expert} />
        </div>
      </div>
    </section>
  );
}
