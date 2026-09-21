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
      <LinksTable
        links={linksQuery.data ?? []}
        onAction={handleAction}
        pendingLinkId={linkAction.isPending ? linkAction.variables?.linkId ?? null : null}
      />
      <div>
        <h2 className="mb-3 text-lg font-semibold">{t("events.heading")}</h2>
        <EventTimeline events={eventsQuery.data ?? []} />
      </div>
    </section>
  );
}
