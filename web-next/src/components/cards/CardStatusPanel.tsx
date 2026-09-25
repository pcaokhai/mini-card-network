"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { CardDetail } from "@/shared/api/cards-client";
import type { StatusKind } from "./cards-model";
import { StatusBadge } from "./StatusBadge";

export type ToggleAction = "block" | "unblock";
export interface AuditEntry {
  action: ToggleAction;
  time: string;
}

interface CardStatusPanelProps {
  card: CardDetail;
  kind: StatusKind;
  expert: boolean;
  audit: readonly AuditEntry[];
  pending: boolean;
  error: string | null;
  onConfirm: (action: ToggleAction) => void;
}

export function CardStatusPanel({ card, kind, expert, audit, pending, error, onConfirm }: CardStatusPanelProps) {
  const t = useTranslations("cards.status");
  const [confirming, setConfirming] = useState(false);
  const last4 = card.maskedPan.slice(-4);
  const action: ToggleAction = kind === "locked" ? "unblock" : "block";
  const Action = action === "block" ? "Block" : "Unblock";
  const desc = expert ? t(`expertDesc.${kind}`, { status: card.status, expiry: card.expiry }) : t(`easyDesc.${kind}`);

  return (
    <section aria-labelledby="cards-status-heading" className="cards-panel cards-status">
      <div className="cards-status__head">
        <h2 id="cards-status-heading" className="cards-panel__heading">
          {t("heading")}
        </h2>
        <StatusBadge card={card} kind={kind} expert={expert} />
      </div>
      <p className="cards-status__desc">{desc}</p>
      {kind !== "expired" && !confirming && (
        <button type="button" className="cards-toggle" data-action={action} onClick={() => setConfirming(true)}>
          {t(action)}
        </button>
      )}
      {kind !== "expired" && confirming && (
        <div className="cards-confirm">
          <p className="cards-confirm__text">{t(`confirm${Action}`, { last4 })}</p>
          <div className="cards-confirm__actions">
            <button
              type="button"
              className="cards-confirm__yes"
              disabled={pending}
              onClick={() => {
                onConfirm(action);
                setConfirming(false);
              }}
            >
              {t(`confirm${Action}Button`)}
            </button>
            <button type="button" className="cards-confirm__no" onClick={() => setConfirming(false)}>
              {t("cancel")}
            </button>
          </div>
        </div>
      )}
      {error !== null && (
        <p role="alert" className="cards-error">
          {t("failed", { detail: error })}
        </p>
      )}
      {audit.map((entry, i) => (
        <p key={i} className="cards-audit">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
            <path d="M5 12l5 5 9-10" />
          </svg>
          {expert ? t("auditExpert", { action: entry.action, last4 }) : t("auditEasy", { action: entry.action, last4, time: entry.time })}
        </p>
      ))}
    </section>
  );
}
