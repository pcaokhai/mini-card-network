"use client";

import { useTranslations } from "next-intl";
import { useCard } from "@/shared/api/cards-client";
import { type CardToken, DISPLAY_CARDS, type DisplayCard } from "./pos-model";

interface CardPickerProps {
  selected: CardToken | null;
  onSelect: (cardToken: CardToken) => void;
}

export function CardPicker({ selected, onSelect }: CardPickerProps) {
  const t = useTranslations("pos.cards");
  return (
    <section aria-label={t("sectionLabel")} className="pos-panel">
      <h2 className="pos-panel__heading">{t("heading")}</h2>
      <div className="pos-cards">
        {DISPLAY_CARDS.map((card) => (
          <CardTile
            key={card.cardToken}
            card={card}
            pressed={selected === card.cardToken}
            onPress={() => onSelect(card.cardToken)}
          />
        ))}
      </div>
    </section>
  );
}

function CardTile({ card, pressed, onPress }: { card: DisplayCard; pressed: boolean; onPress: () => void }) {
  const t = useTranslations("pos.cards");
  // `card` can be missing: openapi-fetch resolves a body-less 404 with neither data nor error.
  const balance = useCard(card.cardRef).data?.card?.availableBalance?.amount;
  return (
    <button type="button" aria-pressed={pressed} data-tag={card.tag} onClick={onPress} className="pos-card">
      <span className="pos-card__tag">{t(`tag.${card.tag}`)}</span>
      <span className="pos-card__pan">•••• {card.last4}</span>
      <span className="pos-card__balance">
        {balance === undefined ? t("balanceUnknown") : t("balance", { amount: balance.toLocaleString("vi-VN") })}
      </span>
    </button>
  );
}
