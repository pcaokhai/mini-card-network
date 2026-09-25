import Link from "next/link";
import { useTranslations } from "next-intl";
import type { CardSummary } from "@/shared/api/cards-client";
import { cardTag, statusKind } from "./cards-model";
import { StatusBadge } from "./StatusBadge";

// The canvas lists cards by their last four digits.
const byLastFour = (cards: CardSummary[]) => cards.toSorted((a, b) => a.maskedPan.slice(-4).localeCompare(b.maskedPan.slice(-4)));

export function CardList({ cards, selectedRef, expert, today }: { cards: CardSummary[]; selectedRef?: string; expert: boolean; today: Date }) {
  const t = useTranslations("cards.list");
  return (
    <section aria-labelledby="cards-list-heading" className="cards-panel cards-list">
      <h2 id="cards-list-heading" className="cards-panel__heading">
        {t("heading")}
      </h2>
      {cards.length === 0 ? (
        <p className="cards-ledger__empty">{t("empty")}</p>
      ) : (
        <ul className="cards-list__items">
          {byLastFour(cards).map((card) => (
            <li key={card.cardRef}>
              <Link
                href={`/cards/${card.cardRef}`}
                aria-current={card.cardRef === selectedRef ? "page" : undefined}
                className="cards-list__item"
              >
                <span aria-hidden="true" className="cards-list__swatch" data-tag={cardTag(card.maskedPan)} />
                <span className="cards-list__who">
                  <span className="cards-list__holder">{card.holderName}</span>
                  <span className="cards-list__pan">•••• {card.maskedPan.slice(-4)}</span>
                </span>
                <StatusBadge card={card} kind={statusKind(card, today)} expert={expert} />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
