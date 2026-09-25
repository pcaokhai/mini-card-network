import { useTranslations } from "next-intl";
import type { Link } from "@/shared/api/network-client";
import type { Overview } from "@/shared/api/overview-client";

const DOT_CLASSES = { ok: "bg-ok", warn: "bg-warn", bad: "bg-bad" } as const;
type Tone = keyof typeof DOT_CLASSES;

function HealthRow({ tone, title, sub, ...rest }: { tone: Tone; title: string; sub: string } & Record<string, unknown>) {
  return (
    <li className="flex items-start gap-3" {...rest}>
      <span aria-hidden className={`mt-1.5 size-2.5 shrink-0 rounded-full ${DOT_CLASSES[tone]}`} />
      <span className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-[13px] text-muted">{sub}</span>
      </span>
    </li>
  );
}

function linkTone(status: Link["status"]): Tone {
  if (status === "SIGNED_ON" || status === "CONNECTED") return "ok";
  return status === "DOWN" ? "bad" : "warn";
}

export function SystemHealthList({ overview, links }: { overview: Overview; links: Link[] }) {
  const t = useTranslations("overview.health");
  const tPill = useTranslations("network.statusPill");

  return (
    <section
      aria-label={t("heading")}
      className="flex flex-col gap-3.5 rounded-card border border-border bg-surface px-5.5 py-5"
    >
      <h2 className="text-[17px] font-semibold">{t("heading")}</h2>
      <ul className="flex flex-col gap-3.5">
        {links.map((link) => (
          <HealthRow
            key={link.linkId}
            tone={linkTone(link.status)}
            title={t("issuerLink")}
            sub={tPill(link.status)}
          />
        ))}
        <HealthRow
          tone={overview.ledgerMatches ? "ok" : "bad"}
          title={t("ledger")}
          sub={overview.ledgerMatches ? t("ledgerOk") : t("ledgerMismatch")}
          data-testid="ledger-health-status"
          data-status={overview.ledgerMatches ? "ok" : "warn"}
        />
      </ul>
    </section>
  );
}
