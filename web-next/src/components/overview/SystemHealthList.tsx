import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import type { Link, SafQueue, SwitchStatus } from "@/shared/api/network-client";
import type { KeyInfo } from "@/shared/api/security-client";

const DOT_CLASSES = { ok: "bg-ok", warn: "bg-warn", bad: "bg-bad" } as const;
type Tone = keyof typeof DOT_CLASSES;

// Keys this close to expiry need an operator to rotate them (docs/03 §8 key lifecycle).
const KEY_WARN_DAYS = 7;
const MS_PER_SECOND = 1000;

interface HealthRowProps {
  id: string;
  tone: Tone;
  title: string;
  sub: string;
  expert: boolean;
}

function HealthRow({ id, tone, title, sub, expert }: HealthRowProps) {
  return (
    <li className="flex items-start gap-3" data-testid={`health-${id}`} data-status={tone}>
      <span aria-hidden className={`mt-1.5 size-2.5 shrink-0 rounded-full ${DOT_CLASSES[tone]}`} />
      <span className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className={expert ? "font-mono text-xs text-muted" : "text-[13px] text-muted"}>{sub}</span>
      </span>
    </li>
  );
}

function secondsSince(iso: string | null | undefined, now: number): number | null {
  if (iso === null || iso === undefined) return null;
  const at = Date.parse(iso);
  return Number.isNaN(at) ? null : Math.max(0, Math.round((now - at) / MS_PER_SECOND));
}

export interface SystemHealthListProps {
  links: Link[];
  saf: SafQueue | undefined;
  switchStatus: SwitchStatus | undefined;
  acquirerKeys: KeyInfo[] | undefined;
  expert: boolean;
  /** Pins the clock for tests; otherwise the list ticks every second so echo ages stay live. */
  now?: number;
}

export function SystemHealthList({ links, saf, switchStatus, acquirerKeys, expert, now: pinnedNow }: SystemHealthListProps) {
  const t = useTranslations("overview.health");
  const [clock, setClock] = useState(() => Date.now());
  useEffect(() => {
    if (pinnedNow !== undefined) return;
    const id = setInterval(() => setClock(Date.now()), MS_PER_SECOND);
    return () => clearInterval(id);
  }, [pinnedNow]);
  const now = pinnedNow ?? clock;
  const pick = (easy: string, tech: string) => (expert ? tech : easy);

  const issuerLink = links.find((l) => l.to.toLowerCase() === "issuer") ?? links[0];
  const zpk = acquirerKeys?.find((k) => k.keyType === "ZPK" && k.status === "ACTIVE");

  return (
    <section
      aria-label={t("heading")}
      className="flex flex-col gap-3.5 rounded-card border border-border bg-surface px-5.5 py-5"
    >
      <h2 className="text-[17px] font-semibold">{t("heading")}</h2>
      <ul className="flex flex-col gap-3.5">
        {issuerLink !== undefined && (() => {
          const up = issuerLink.status === "SIGNED_ON" || issuerLink.status === "CONNECTED";
          const seconds = secondsSince(issuerLink.lastEchoAt, now);
          return (
            <HealthRow
              id="link"
              tone={up ? "ok" : "bad"}
              title={t("issuerLink")}
              sub={pick(
                up ? t("linkEasyUp", { seconds: seconds ?? 0 }) : t("linkEasyDown"),
                t("linkTech", { status: issuerLink.status, seconds: seconds ?? 0 }),
              )}
              expert={expert}
            />
          );
        })()}
        {saf !== undefined && (
          <HealthRow
            id="saf"
            tone={saf.deadCount > 0 ? "bad" : saf.depth > 0 ? "warn" : "ok"}
            title={t("saf")}
            sub={pick(
              saf.depth === 0 ? t("safEasyEmpty") : t("safEasyPending", { depth: saf.depth }),
              t("safTech", { depth: saf.depth, dead: saf.deadCount }),
            )}
            expert={expert}
          />
        )}
        {switchStatus !== undefined && (
          <HealthRow
            id="stip"
            tone={switchStatus.circuit === "OPEN" ? "bad" : switchStatus.stipActive ? "warn" : "ok"}
            title={t("stip")}
            sub={pick(
              switchStatus.stipActive ? t("stipEasyOn") : t("stipEasyOff"),
              t("stipTech", { stip: switchStatus.stipActive ? "ON" : "OFF", circuit: switchStatus.circuit }),
            )}
            expert={expert}
          />
        )}
        {zpk !== undefined && (
          <HealthRow
            id="key"
            tone={zpk.daysRemaining <= 0 ? "bad" : zpk.daysRemaining <= KEY_WARN_DAYS ? "warn" : "ok"}
            title={t("key")}
            sub={pick(
              zpk.daysRemaining <= 0 ? t("keyEasyExpired") : t("keyEasyValid", { days: zpk.daysRemaining }),
              t("keyTech", { kcv: zpk.kcv, days: zpk.daysRemaining }),
            )}
            expert={expert}
          />
        )}
      </ul>
    </section>
  );
}
