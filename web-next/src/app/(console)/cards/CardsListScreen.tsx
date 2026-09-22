"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import { useCards } from "@/shared/api/cards-client";

const STATUS_CLASSES: Record<string, string> = {
  ACTIVE: "bg-ok-soft text-ok",
  BLOCKED: "bg-bad-soft text-bad",
  LOST: "bg-bad-soft text-bad",
  STOLEN: "bg-bad-soft text-bad",
  EXPIRED: "bg-canvas text-muted",
  PIN_BLOCKED: "bg-warn-soft text-warn",
};

export function CardsListScreen() {
  const t = useTranslations("cards");
  const { data: cards } = useCards();

  return (
    <section aria-labelledby="cards-heading" className="space-y-6">
      <h1 id="cards-heading" className="text-2xl font-bold">
        {t("title")}
      </h1>
      {cards !== undefined && cards.length === 0 && <p className="text-sm text-muted">{t("list.empty")}</p>}
      <table className="w-full text-sm">
        <tbody>
          {cards?.map((card) => (
            <tr key={card.cardRef} className="border-t border-border">
              <td className="py-2">
                <Link href={`/cards/${card.cardRef}`} className="font-medium text-accent hover:underline">
                  {card.maskedPan}
                </Link>
              </td>
              <td className="py-2">{card.holderName}</td>
              <td className="py-2">
                <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${STATUS_CLASSES[card.status]}`}>
                  {t(`status.${card.status}`)}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
