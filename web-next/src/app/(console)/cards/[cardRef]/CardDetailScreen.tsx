"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { CardVisual } from "@/components/cards/CardVisual";
import { BalanceLines } from "@/components/cards/BalanceLines";
import { LimitsSliders } from "@/components/cards/LimitsSliders";
import { LedgerTable } from "@/components/cards/LedgerTable";
import { BlockConfirmDialog } from "@/components/cards/BlockConfirmDialog";
import { AuditLine, type AuditEntry } from "@/components/cards/AuditLine";
import {
  useBlockCard,
  useCard,
  useCardLedger,
  useUnblockCard,
  useUpdateLimits,
  PreconditionFailedError,
  type BlockReason,
  type CardLimits,
} from "@/shared/api/cards-client";
import "@/components/cards/cards.css";

export function CardDetailScreen({ cardRef }: { cardRef: string }) {
  const t = useTranslations("cards");
  const { data } = useCard(cardRef);
  const { data: ledger } = useCardLedger(cardRef);
  const updateLimits = useUpdateLimits(cardRef);
  const blockCard = useBlockCard(cardRef);
  const unblockCard = useUnblockCard(cardRef);
  const [showBlockDialog, setShowBlockDialog] = useState(false);
  const [staleError, setStaleError] = useState(false);
  const [audit, setAudit] = useState<AuditEntry[]>([]);

  if (data === undefined) return null;
  const { card, etag } = data;

  function pushAudit(action: AuditEntry["action"]) {
    setAudit((prev) => [...prev, { actor: "operator@lab", action, occurredAt: new Date().toISOString() }]);
  }

  function handleSaveLimits(limits: CardLimits) {
    setStaleError(false);
    updateLimits.mutate(
      { limits, ifMatch: etag },
      {
        onSuccess: () => pushAudit("LIMITS_UPDATED"),
        onError: (error) => {
          if (error instanceof PreconditionFailedError) setStaleError(true);
        },
      },
    );
  }

  function handleBlockConfirm(reason: BlockReason) {
    blockCard.mutate(reason, { onSuccess: () => pushAudit("CARD_BLOCKED") });
    setShowBlockDialog(false);
  }

  function handleUnblockConfirm() {
    unblockCard.mutate(undefined, { onSuccess: () => pushAudit("CARD_UNBLOCKED") });
    setShowBlockDialog(false);
  }

  const isBlocked = card.status === "BLOCKED";

  return (
    <section aria-labelledby="card-detail-heading" className="space-y-6">
      <h1 id="card-detail-heading" className="text-2xl font-bold">
        {card.maskedPan}
      </h1>
      <div className="relative">
        <CardVisual card={card} />
        {isBlocked && (
          <div
            role="status"
            className="card-lock-overlay absolute inset-0 flex items-center justify-center rounded-card bg-ink/70 text-sm font-semibold text-white"
          >
            {t("status.BLOCKED")}
          </div>
        )}
      </div>
      <BalanceLines ledgerBalance={card.ledgerBalance} availableBalance={card.availableBalance} holds={card.holds} />
      {staleError && <p role="alert">{t("errors.staleCard")}</p>}
      <LimitsSliders limits={card.limits} usedToday={card.usedToday} onSave={handleSaveLimits} />
      {showBlockDialog ? (
        <BlockConfirmDialog
          action={isBlocked ? "unblock" : "block"}
          onConfirm={isBlocked ? handleUnblockConfirm : handleBlockConfirm}
          onCancel={() => setShowBlockDialog(false)}
        />
      ) : (
        <button
          type="button"
          onClick={() => setShowBlockDialog(true)}
          className="rounded-lg border border-border px-4 py-2 text-sm font-semibold"
        >
          {t(isBlocked ? "actions.unblock" : "actions.block")}
        </button>
      )}
      {audit.map((entry, i) => (
        <AuditLine key={i} entry={entry} />
      ))}
      {ledger !== undefined && <LedgerTable entries={ledger} />}
    </section>
  );
}
