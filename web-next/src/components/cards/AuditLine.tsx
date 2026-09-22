import { useTranslations } from "next-intl";

export interface AuditEntry {
  actor: string;
  action: "CARD_BLOCKED" | "CARD_UNBLOCKED" | "LIMITS_UPDATED";
  occurredAt: string;
}

export function AuditLine({ entry }: { entry: AuditEntry }) {
  const t = useTranslations("cards.audit");
  return (
    <p className="text-xs text-muted">
      {t(entry.action, { actor: entry.actor, at: new Date(entry.occurredAt).toLocaleString() })}
    </p>
  );
}
