"use client";

import { useTranslations } from "next-intl";
import { useLinkAction, useLinks, useNetworkEvents } from "@/shared/api/network-client";
import { EventTimeline } from "./EventTimeline";
import { LinksTable } from "./LinksTable";
import type { LinkAction } from "@/shared/api/network-client";

export function NetworkScreen() {
  const t = useTranslations("network");
  const linksQuery = useLinks();
  const eventsQuery = useNetworkEvents();
  const linkAction = useLinkAction();

  function handleAction(linkId: string, action: LinkAction) {
    linkAction.mutate({ linkId, action });
  }

  return (
    <section aria-labelledby="network-heading" className="space-y-6">
      <h1 id="network-heading" className="text-2xl font-bold">{t("title")}</h1>
      <section aria-label={t("linksHeading")} className="rounded-card border border-border bg-surface p-5">
        <h2 className="mb-3 text-base font-semibold">{t("linksHeading")}</h2>
        <LinksTable
          links={linksQuery.data ?? []}
          onAction={handleAction}
          pendingLinkId={linkAction.isPending ? linkAction.variables?.linkId ?? null : null}
        />
      </section>
      <section aria-label={t("events.heading")} className="rounded-card border border-border bg-surface p-5">
        <h2 className="mb-3 text-base font-semibold">{t("events.heading")}</h2>
        <EventTimeline events={eventsQuery.data ?? []} />
      </section>
    </section>
  );
}
