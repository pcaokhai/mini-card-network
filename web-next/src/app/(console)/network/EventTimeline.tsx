import { useTranslations } from "next-intl";
import type { NetworkEvent } from "@/shared/api/network-client";

const SEVERITY_BORDER: Record<NetworkEvent["severity"], string> = {
  INFO: "border-accent",
  OK: "border-ok",
  WARN: "border-warn",
  ERROR: "border-bad",
};

export function EventTimeline({ events }: { events: NetworkEvent[] }) {
  const t = useTranslations("network.events");
  if (events.length === 0) return <p className="text-sm text-muted">{t("empty")}</p>;

  const sorted = [...events].sort((a, b) => b.occurredAt.localeCompare(a.occurredAt));
  return (
    <ul className="flex flex-col gap-3">
      {sorted.map((event) => (
        <li key={event.id} data-severity={event.severity} className={`border-l-2 pl-3 ${SEVERITY_BORDER[event.severity]}`}>
          <time dateTime={event.occurredAt} className="text-xs text-muted">
            {new Date(event.occurredAt).toLocaleTimeString()}
          </time>
          <p className="font-medium">{event.easyText}</p>
          <p className="font-mono text-xs text-muted">{event.technicalText}</p>
        </li>
      ))}
    </ul>
  );
}
