import { useTranslations } from "next-intl";
import type { CardDetail } from "@/shared/api/cards-client";

const STATUS_CLASSES: Record<CardDetail["status"], string> = {
  ACTIVE: "bg-ok-soft text-ok",
  BLOCKED: "bg-bad-soft text-bad",
  LOST: "bg-bad-soft text-bad",
  STOLEN: "bg-bad-soft text-bad",
  EXPIRED: "bg-canvas text-muted",
  PIN_BLOCKED: "bg-warn-soft text-warn",
};

export function CardVisual({ card }: { card: Pick<CardDetail, "maskedPan" | "holderName" | "status" | "expiry"> }) {
  const t = useTranslations("cards.status");
  return (
    <div className="rounded-card bg-ink px-6 py-5 text-white">
      <div className="flex items-start justify-between">
        <span className="font-mono text-lg tracking-wider">{card.maskedPan}</span>
        <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${STATUS_CLASSES[card.status]}`}>
          {t(card.status)}
        </span>
      </div>
      <div className="mt-6 flex items-end justify-between text-sm">
        <span>{card.holderName}</span>
        <span className="font-mono">{card.expiry}</span>
      </div>
    </div>
  );
}
