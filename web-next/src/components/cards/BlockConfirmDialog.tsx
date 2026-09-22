"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { BlockReason } from "@/shared/api/cards-client";

const REASONS: BlockReason[] = ["CUSTOMER_REQUEST", "LOST", "STOLEN", "FRAUD_SUSPECTED"];

export function BlockConfirmDialog({
  action,
  onConfirm,
  onCancel,
}: {
  action: "block" | "unblock";
  onConfirm: (reason: BlockReason) => void;
  onCancel: () => void;
}) {
  const t = useTranslations("cards.blockDialog");
  const [reason, setReason] = useState<BlockReason>("CUSTOMER_REQUEST");

  return (
    <div role="group" aria-label={t(action)} className="rounded-lg border border-border bg-canvas p-4">
      <p className="mb-3 text-sm">{t(`${action}Prompt`)}</p>
      {action === "block" && (
        <label className="mb-3 block text-sm">
          {t("reason")}
          <select
            value={reason}
            onChange={(e) => setReason(e.target.value as BlockReason)}
            className="ml-2 rounded-lg border border-border bg-surface px-2 py-1"
          >
            {REASONS.map((r) => (
              <option key={r} value={r}>
                {t(`reasons.${r}`)}
              </option>
            ))}
          </select>
        </label>
      )}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={() => onConfirm(reason)}
          className="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-white"
        >
          {t("confirm")}
        </button>
        <button type="button" onClick={onCancel} className="rounded-lg px-4 py-2 text-sm font-semibold text-muted">
          {t("cancel")}
        </button>
      </div>
    </div>
  );
}
