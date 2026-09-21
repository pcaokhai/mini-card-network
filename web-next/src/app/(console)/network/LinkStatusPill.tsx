import { useTranslations } from "next-intl";
import type { Link } from "@/shared/api/network-client";

const STATUS_CLASSES: Record<Link["status"], string> = {
  SIGNED_ON: "bg-ok-soft text-ok",
  CONNECTED: "bg-accent-soft text-accent",
  DOWN: "bg-bad-soft text-bad",
  DISCONNECTED: "bg-canvas text-muted",
};

export function LinkStatusPill({ status }: { status: Link["status"] }) {
  const t = useTranslations("network.statusPill");
  return (
    <span
      data-testid="link-status-pill"
      data-status={status}
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ${STATUS_CLASSES[status]}`}
    >
      <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-current" />
      {t(status)}
    </span>
  );
}
