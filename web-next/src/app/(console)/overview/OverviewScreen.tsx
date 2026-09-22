"use client";

import { useTranslations } from "next-intl";
import { useOverview } from "@/shared/api/overview-client";
import { useLinks } from "@/shared/api/network-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import { KpiCards } from "@/components/overview/KpiCards";
import { ThroughputChart } from "@/components/overview/ThroughputChart";
import { DeclineReasonsBreakdown } from "@/components/overview/DeclineReasonsBreakdown";
import { SystemHealthList } from "@/components/overview/SystemHealthList";
import { CutoverCountdown } from "@/components/overview/CutoverCountdown";
import { LiveFeed } from "@/components/overview/LiveFeed";
import "@/components/overview/overview.css";

export function OverviewScreen() {
  const t = useTranslations("overview");
  const overviewQuery = useOverview();
  const linksQuery = useLinks();
  const expertMode = useDisplayMode((s) => s.mode === "expert");

  if (!overviewQuery.data) return null;

  return (
    <section aria-labelledby="overview-heading" className="space-y-6">
      <h1 id="overview-heading" className="text-2xl font-bold">
        {t("title")}
      </h1>
      <KpiCards overview={overviewQuery.data} />
      <div className="grid gap-6 lg:grid-cols-2">
        <ThroughputChart throughput={overviewQuery.data.throughput} />
        <DeclineReasonsBreakdown declineReasons={overviewQuery.data.declineReasons} />
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <SystemHealthList overview={overviewQuery.data} links={linksQuery.data ?? []} />
        <CutoverCountdown />
      </div>
      <LiveFeed expertMode={expertMode} />
    </section>
  );
}
