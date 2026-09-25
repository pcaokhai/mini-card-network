import { useTranslations } from "next-intl";
import type { CardSummary } from "@/shared/api/cards-client";
import { LockIcon } from "@/shared/ui/icons";
import { cardTag } from "./cards-model";

/** Only the last four digits ever reach the browser's markup (web-next/CLAUDE.md, card data). */
export function CardVisual({ card, locked }: { card: Pick<CardSummary, "maskedPan" | "holderName" | "expiry">; locked: boolean }) {
  const t = useTranslations("cards.visual");
  return (
    <div className="cards-visual" data-tag={cardTag(card.maskedPan)}>
      <div className="cards-visual__row">
        <span className="cards-visual__brand">{t("brand")}</span>
        <span aria-hidden="true" className="cards-visual__chip" />
      </div>
      <div className="cards-visual__pan">•••• •••• •••• {card.maskedPan.slice(-4)}</div>
      <div className="cards-visual__meta">
        <span>{card.holderName}</span>
        <span>{t("expires", { expiry: card.expiry })}</span>
      </div>
      {locked && (
        <div role="status" className="cards-lock">
          <span className="cards-lock__icon">
            <LockIcon width={24} height={24} strokeWidth={2} />
          </span>
          <span>{t("locked")}</span>
        </div>
      )}
    </div>
  );
}
