"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { CardAuditEntry, CardDetail } from "@/shared/api/cards-client";
import type { StatusKind } from "./cards-model";
import { StatusBadge } from "./StatusBadge";

export type ToggleAction = "block" | "unblock";

const clock = new Intl.DateTimeFormat("vi-VN", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });

interface CardStatusPanelProps {
  card: CardDetail;
  kind: StatusKind;
  expert: boolean;
  /** The issuer's audit entries for this card, newest first. */
  audit: readonly CardAuditEntry[];
  pending: boolean;
  error: string | null;
  onConfirm: (action: ToggleAction) => void;
}

export function CardStatusPanel({ card, kind, expert, audit, pending, error, onConfirm }: CardStatusPanelProps) {
  const t = useTranslations("cards.status");
  const [confirming, setConfirming] = useState(false);
  const last4 = card.maskedPan.slice(-4);
  const action: ToggleAction = kind === "locked" ? "unblock" : "block";
  // Only an operator's block can be undone; the issuer refuses to unblock LOST, STOLEN or
  // PIN_BLOCKED (409), so the screen doesn't offer it (CARDS-G5).
  const canToggle = kind !== "expired" && (action === "block" || card.status === "BLOCKED");
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
      {kind === "locked" && !canToggle && <p className="cards-status__desc">{t("noUnblock")}</p>}
      {canToggle && !confirming && (
        <button type="button" className="cards-toggle" data-action={action} onClick={() => setConfirming(true)}>
          {t(action)}
        </button>
      )}
      {canToggle && confirming && (
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
      {audit.length > 0 && (
        <ul aria-label={t("auditHeading")} className="cards-audit-list">
          {audit.map((entry) => {
            const params = { action: entry.action, last4, actor: entry.actor, time: clock.format(new Date(entry.occurredAt)) };
            return (
              <li key={entry.auditId} className="cards-audit">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
                  <path d="M5 12l5 5 9-10" />
                </svg>
                {expert ? t("auditExpert", params) : t("auditEasy", params)}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
