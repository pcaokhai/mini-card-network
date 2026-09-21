import { useTranslations } from "next-intl";
import { LinkStatusPill } from "./LinkStatusPill";
import type { Link, LinkAction } from "@/shared/api/network-client";

interface LinksTableProps {
  links: Link[];
  onAction: (linkId: string, action: LinkAction) => void;
  pendingLinkId: string | null;
}

export function LinksTable({ links, onAction, pendingLinkId }: LinksTableProps) {
  const t = useTranslations("network");
  return (
    <table className="w-full text-sm">
      <thead>
        <tr>
          <th scope="col" className="text-left font-medium text-muted">{t("table.link")}</th>
          <th scope="col" className="text-left font-medium text-muted">{t("table.status")}</th>
          <th scope="col" className="text-left font-medium text-muted">{t("table.lastEcho")}</th>
          <th scope="col" className="text-left font-medium text-muted">{t("table.p99")}</th>
          <th scope="col" className="text-left font-medium text-muted">{t("table.inFlight")}</th>
          <th scope="col" />
        </tr>
      </thead>
      <tbody>
        {links.map((link) => {
          const isPending = pendingLinkId === link.linkId;
          return (
            <tr key={link.linkId} aria-label={link.linkId} className="border-t border-border">
              <td className="py-2 font-mono">{link.linkId}</td>
              <td className="py-2"><LinkStatusPill status={link.status} /></td>
              <td className="py-2 font-mono text-muted">{link.lastEchoAt ?? "—"}</td>
              <td className="py-2 font-mono text-muted">{link.p99LatencyMs != null ? `${link.p99LatencyMs}ms` : "—"}</td>
              <td className="py-2 font-mono text-muted">{link.inFlight}</td>
              <td className="py-2">
                <div className="flex gap-2">
                  <button type="button" disabled={isPending} className="rounded-card border border-border px-2 py-1 hover:bg-canvas disabled:opacity-50" onClick={() => onAction(link.linkId, "echo")}>
                    {t("actions.echo")}
                  </button>
                  <button type="button" disabled={isPending} className="rounded-card border border-border px-2 py-1 hover:bg-canvas disabled:opacity-50" onClick={() => onAction(link.linkId, "sign-on")}>
                    {t("actions.signOn")}
                  </button>
                  <button type="button" disabled={isPending} className="rounded-card border border-border px-2 py-1 hover:bg-canvas disabled:opacity-50" onClick={() => onAction(link.linkId, "sign-off")}>
                    {t("actions.signOff")}
                  </button>
                </div>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
