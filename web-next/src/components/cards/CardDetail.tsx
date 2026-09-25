"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useBlockCard, useCard, useCardLedger, useUnblockCard } from "@/shared/api/cards-client";
import { BalancePanel } from "./BalancePanel";
import { type AuditEntry, CardStatusPanel, type ToggleAction } from "./CardStatusPanel";
import { CardVisual } from "./CardVisual";
import { LedgerPanel } from "./LedgerPanel";
import { LimitsPanel } from "./LimitsPanel";
import { statusKind } from "./cards-model";
import { problemDetail } from "./problem-detail";

const clock = new Intl.DateTimeFormat("vi-VN", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });

export function CardDetail({ cardRef, expert, today }: { cardRef: string; expert: boolean; today: Date }) {
  const t = useTranslations("cards");
  const cardQuery = useCard(cardRef);
  const ledger = useCardLedger(cardRef);
  const block = useBlockCard(cardRef);
  const unblock = useUnblockCard(cardRef);
  // The issuer writes audit_log itself but has no read API yet; this echoes what this session did.
  const [audit, setAudit] = useState<AuditEntry[]>([]);

  if (cardQuery.isError) {
    return (
      <p role="alert" className="cards-error">
        {t("loadFailed")}
      </p>
    );
  }
  // `card` can be missing: openapi-fetch resolves a body-less 404 with neither data nor error.
  const card = cardQuery.data?.card;
  if (!card) return null;
  const kind = statusKind(card, today);

  function toggle(action: ToggleAction) {
    const logged = { onSuccess: () => setAudit((a) => [...a, { action, time: clock.format(new Date()) }]) };
    if (action === "block") block.mutate("CUSTOMER_REQUEST", logged);
    else unblock.mutate(undefined, logged);
  }
  const toggleError = block.error ?? unblock.error;

  return (
    <div className="cards-detail">
      <div className="cards-top">
        <CardVisual card={card} locked={kind === "locked"} />
        <CardStatusPanel
          card={card}
          kind={kind}
          expert={expert}
          audit={audit}
          pending={block.isPending || unblock.isPending}
          error={toggleError ? problemDetail(toggleError) : null}
          onConfirm={toggle}
        />
      </div>
      <div className="cards-pair">
        <BalancePanel card={card} expert={expert} />
        <LimitsPanel card={card} etag={cardQuery.data?.etag ?? null} expert={expert} onReload={() => void cardQuery.refetch()} />
      </div>
      <LedgerPanel entries={ledger.data ?? []} expert={expert} />
    </div>
  );
}
