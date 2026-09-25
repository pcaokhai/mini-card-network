import { useTranslations } from "next-intl";
import { LinkStatusPill } from "@/app/(console)/network/LinkStatusPill";
import type { Link } from "@/shared/api/network-client";
import type { Overview } from "@/shared/api/overview-client";

export function SystemHealthList({ overview, links }: { overview: Overview; links: Link[] }) {
  const t = useTranslations("overview.health");
  return (
    <div className="rounded-card border border-border bg-surface p-4">
      <h2 className="mb-3 text-lg font-semibold">{t("heading")}</h2>
      <ul className="space-y-2">
        <li className="flex items-center justify-between" data-status={overview.ledgerMatches ? "ok" : "warn"}>
          <span>{t("ledger")}</span>
          <span
            data-testid="ledger-health-status"
            className={overview.ledgerMatches ? "text-ok" : "text-bad"}
          >
            {overview.ledgerMatches ? "OK" : "WARN"}
          </span>
        </li>
        {links.map((link) => (
          <li key={link.linkId} className="flex items-center justify-between">
            <span>{link.linkId}</span>
            <LinkStatusPill status={link.status} />
          </li>
        ))}
      </ul>
    </div>
  );
}
