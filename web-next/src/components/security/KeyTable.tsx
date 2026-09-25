"use client";

import { useTranslations } from "next-intl";
import type { KeyInfo } from "@/shared/api/security-client";
import { keyRows } from "./security-model";

type KeyTableProps = {
  keys: readonly KeyInfo[];
  expert: boolean;
  canRotate: boolean;
  onRotate: () => void;
  /** A rotation's new KCV: it flips in once the key list carries it, not before. */
  newKcv: string | null;
};

/** Every key with its check value, how much of its lifetime is left and whether it is due for rotation. */
export function KeyTable({ keys, expert, canRotate, onRotate, newKcv }: KeyTableProps) {
  const t = useTranslations("security.keys");
  const mode = expert ? "expert" : "easy";

  return (
    <section aria-label={t("region")} className="security-panel security-keys">
      <div className="security-keys__title">
        <h2 className="security-panel__heading">{t("heading")}</h2>
        <span className="security-note">{t(`note.${mode}`)}</span>
      </div>
      <table className="security-keys__table">
        <thead>
          <tr className="security-keys__grid security-keys__head">
            <th scope="col">{t("columns.key")}</th>
            <th scope="col">{t("columns.kcv")}</th>
            <th scope="col">{t("columns.lifetime")}</th>
            <th scope="col">{t("columns.status")}</th>
            <th scope="col">
              <span className="sr-only">{t("columns.action")}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {keyRows(keys).map((row) => {
            const name = t(`types.${row.keyType}.name`);
            return (
              <tr key={row.keyType} className="security-keys__grid security-keys__row">
                <td className="security-keys__key">
                  <span className="security-keys__name">{expert ? `${row.keyType} · ${name}` : name}</span>
                  <span className="security-note">
                    {expert ? t("expertNote", { days: row.lifetimeDays }) : t(`types.${row.keyType}.note`)}
                  </span>
                </td>
                <td>
                  <span key={row.kcv} className="security-kcv" data-flip={row.kcv === newKcv ? "true" : undefined}>
                    {row.kcv}
                  </span>
                </td>
                <td className="security-keys__life">
                  <span className="security-bar" data-tone={row.tone}>
                    <span className="security-bar__fill" style={{ transform: `scaleX(${row.percent / 100})` }} />
                  </span>
                  <span className="security-note">{t("daysLeft", { days: row.daysRemaining })}</span>
                </td>
                <td>
                  <span className="security-badge" data-tone={row.tone}>
                    {t(`status.${mode}.${row.status}`)}
                  </span>
                </td>
                <td>
                  {canRotate && row.keyType === "ZPK" && (
                    <button type="button" className="security-button" data-variant="outline" onClick={onRotate}>
                      {t("rotateNow")}
                    </button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}
