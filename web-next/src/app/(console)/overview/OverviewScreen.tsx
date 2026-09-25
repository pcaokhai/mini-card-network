"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import { useOverview } from "@/shared/api/overview-client";
import { useLinks, useSafQueue, useSwitchStatus } from "@/shared/api/network-client";
import { useAcquirerKeys } from "@/shared/api/security-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import { KpiCards } from "@/components/overview/KpiCards";
import { DeclineReasonsBreakdown } from "@/components/overview/DeclineReasonsBreakdown";
import { SystemHealthList } from "@/components/overview/SystemHealthList";
import { CutoverCountdown } from "@/components/overview/CutoverCountdown";
import { LiveFeed } from "@/components/overview/LiveFeed";
import "@/components/overview/overview.css";

function todayLabel(): string {
  return new Intl.DateTimeFormat("vi-VN", {
    weekday: "long",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  })
    .format(new Date())
    .replace(/^./, (c) => c.toUpperCase());
}

export function OverviewScreen() {
  const t = useTranslations("overview");
  const overviewQuery = useOverview();
  const linksQuery = useLinks();
  const safQuery = useSafQueue();
  const switchQuery = useSwitchStatus();
  const keysQuery = useAcquirerKeys();
  const expertMode = useDisplayMode((s) => s.mode === "expert");
  if (!overviewQuery.data) return null;

  return (
    <section aria-labelledby="overview-heading" className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 id="overview-heading" className="text-[30px] font-bold tracking-[-0.01em]">
            {t("title")}
          </h1>
          {/* The date is the viewer's local day; the server has no timezone to match it. */}
          <p className="mt-1.5 text-[15px] text-muted" suppressHydrationWarning>
            {`${todayLabel()} · ${t("subtitle")}`}
          </p>
        </div>
        <div className="flex gap-2.5">
          <Link
            href="/lab/chaos"
            className="inline-flex h-11 items-center rounded-[10px] border border-[#D6D3CA] bg-surface px-4.5 text-sm font-semibold"
          >
            {t("actions.openChaosLab")}
          </Link>
          <Link
            href="/pos"
            className="inline-flex h-11 items-center rounded-[10px] bg-accent px-4.5 text-sm font-semibold text-white"
          >
            {t("actions.createTestTransaction")}
          </Link>
        </div>
      </div>

      <KpiCards overview={overviewQuery.data} />

      {/* Two columns only once the expert feed (664px of fixed columns) fits beside the 380px rail. */}
      <div className="grid gap-5 min-[1400px]:grid-cols-[minmax(0,1fr)_380px]">
        <LiveFeed expertMode={expertMode} throughput={overviewQuery.data.throughput} />
        <div className="flex flex-col gap-5">
          <SystemHealthList
            links={linksQuery.data ?? []}
            saf={safQuery.data}
            switchStatus={switchQuery.data}
            acquirerKeys={keysQuery.data}
            expert={expertMode}
          />
          <DeclineReasonsBreakdown declineReasons={overviewQuery.data.declineReasons} expert={expertMode} />
          <CutoverCountdown />
        </div>
      </div>
    </section>
  );
}
