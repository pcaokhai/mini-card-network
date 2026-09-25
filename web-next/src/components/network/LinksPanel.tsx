import { useTranslations } from "next-intl";
import { echoAge, endpointName, linkTone } from "./network-model";
import type { Link } from "@/shared/api/network-client";

interface LinksPanelProps {
  links: Link[];
  expert: boolean;
  now: number;
  pendingLinkId: string | null;
  onEcho: (linkId: string) => void;
}

/** MCN-205-AC2: one row per ISO link with status, latency, last echo and a "check now" echo. */
export function LinksPanel({ links, expert, now, pendingLinkId, onEcho }: LinksPanelProps) {
  const t = useTranslations("network.links");
  const pick = (easy: string, tech: string) => t(expert ? tech : easy);
  const label = (id: string) => (id === "switch" ? t("switch") : endpointName(id));

  return (
    <section aria-label={t("ariaLabel")} className="net-card gap-1">
      <h2 className="mb-2">{t("heading")}</h2>
      {links.length === 0 ? (
        <p className="net-desc">{t("empty")}</p>
      ) : (
        <table className="net-links">
          <thead>
            <tr>
              <th scope="col">{t("colLink")}</th>
              <th scope="col">{t("colStatus")}</th>
              <th scope="col">{pick("colLatencyEasy", "colLatencyTech")}</th>
              <th scope="col">{pick("colEchoEasy", "colEchoTech")}</th>
              <td />
            </tr>
          </thead>
          <tbody>
            {links.map((link) => {
              const address = `${link.from} → ${link.to}`;
              const name = expert ? address : `${label(link.from)} → ${label(link.to)}`;
              const age = echoAge(link, now);
              return (
                <tr key={link.linkId} aria-label={name}>
                  <td className="net-links__name">
                    <span className="font-semibold">{name}</span>
                    <span className="net-links__addr">{expert ? t("protocol") : address}</span>
                  </td>
                  <td>
                    <span className="net-badge" data-tone={linkTone(link.status)}>
                      {expert ? link.status : t(`status.${link.status}`)}
                    </span>
                  </td>
                  <td className="tabular-nums">
                    {link.status === "DOWN" || link.p99LatencyMs == null ? "—" : t("latency", { ms: link.p99LatencyMs })}
                  </td>
                  <td className="net-links__echo">{t(`echo.${age.key}`, { n: age.n })}</td>
                  <td>
                    <button type="button" className="net-ping" disabled={pendingLinkId === link.linkId} onClick={() => onEcho(link.linkId)}>
                      {pick("pingEasy", "pingTech")}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </section>
  );
}
