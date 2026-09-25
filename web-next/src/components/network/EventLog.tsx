import { useTranslations } from "next-intl";
import { todaysEvents, type Tone } from "./network-model";
import type { NetworkEvent } from "@/shared/api/network-client";

const SEVERITY_TONE: Record<NetworkEvent["severity"], Tone> = { OK: "ok", INFO: "info", WARN: "warn", ERROR: "bad" };

const TIME_FORMAT = new Intl.DateTimeFormat("vi-VN", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });

/** MCN-205-AC3: today's network events, newest first; new rows fade up as they arrive. */
export function EventLog({ events, expert, now }: { events: NetworkEvent[]; expert: boolean; now: number }) {
  const t = useTranslations("network.events");
  const rows = todaysEvents(events, now);

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
              <span className="net-event__text">{expert ? event.technicalText : event.easyText}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
