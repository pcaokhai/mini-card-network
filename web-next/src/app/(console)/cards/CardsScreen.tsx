"use client";

import { useTranslations } from "next-intl";
import { CardDetail } from "@/components/cards/CardDetail";
import { CardList } from "@/components/cards/CardList";
import { useCards } from "@/shared/api/cards-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import "@/components/cards/cards.css";

/** One screen for /cards and /cards/[cardRef]: the list beside the selected card (first card by default). */
export function CardsScreen({ cardRef }: { cardRef?: string }) {
  const t = useTranslations("cards");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const cards = useCards();
  const selected = cardRef ?? cards.data?.[0]?.cardRef;
  const today = new Date();

  return (
    <section aria-labelledby="cards-heading" className="cards-root flex flex-col gap-5">
      <div>
        <h1 id="cards-heading" className="text-[30px] font-bold tracking-[-0.01em]">
          {t("title")}
        </h1>
        <p className="mt-1.5 text-[15px] text-muted">{t("subtitle")}</p>
      </div>
      {cards.isError && (
        <p role="alert" className="cards-error">
          {t("loadFailed")}
        </p>
      )}
      <div className="cards-columns">
        {cards.data && <CardList cards={cards.data} selectedRef={selected} expert={expert} today={today} />}
        {selected !== undefined && <CardDetail key={selected} cardRef={selected} expert={expert} today={today} />}
      </div>
    </section>
  );
}
