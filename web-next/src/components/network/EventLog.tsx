import { useState } from "react";
import { useTranslations } from "next-intl";
import { gatewayEventKey, todaysEvents, type Tone } from "./network-model";
import type { NetworkEvent } from "@/shared/api/network-client";

const SEVERITY_TONE: Record<NetworkEvent["severity"], Tone> = { OK: "ok", INFO: "info", WARN: "warn", ERROR: "bad" };

// The canvas's log holds five rows (two outage events over the three of the day).
const VISIBLE_ROWS = 5;

const TIME_FORMAT = new Intl.DateTimeFormat("vi-VN", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });

/** MCN-205-AC3: today's network events, newest first; new rows fade up as they arrive. */
export function EventLog({ events, expert, now }: { events: NetworkEvent[]; expert: boolean; now: number }) {
  const t = useTranslations("network.events");
  const [expanded, setExpanded] = useState(false);
  const all = todaysEvents(events, now);
  const rows = expanded ? all : all.slice(0, VISIBLE_ROWS);
  const hidden = all.length - rows.length;
  const easyText = (event: NetworkEvent) => {
    const key = gatewayEventKey(event.easyText);
    return key ? t(`known.${key}`) : event.easyText;
  };

  return (
    <section aria-label={t("ariaLabel")} className="net-card gap-1">
      <h2 className="mb-2">{t("heading")}</h2>
      {rows.length === 0 ? (
        <p className="net-desc">{t("empty")}</p>
      ) : (
        <ol>
          {rows.map((event) => (
            <li key={event.id} className="net-event" data-severity={event.severity}>
              <time dateTime={event.occurredAt}>{TIME_FORMAT.format(new Date(event.occurredAt))}</time>
              <span aria-hidden className="net-event__dot" data-tone={SEVERITY_TONE[event.severity]} />
              <span className="net-event__text">{expert ? event.technicalText : easyText(event)}</span>
            </li>
          ))}
        </ol>
      )}
      {hidden > 0 && (
        <button type="button" className="net-more" onClick={() => setExpanded(true)}>
          {t("showMore", { count: hidden })}
        </button>
      )}
    </section>
  );
}
