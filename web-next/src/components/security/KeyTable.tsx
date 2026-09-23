"use client";

import { useTranslations } from "next-intl";
import { useAcquirerKeys, useIssuerKeys } from "@/shared/api/security-client";
import { KeyLifetimeBar } from "./KeyLifetimeBar";

export function KeyTable() {
  const t = useTranslations("security.keys");
  const { data: acquirerKeys } = useAcquirerKeys();
  const { data: issuerKeys } = useIssuerKeys();

  const rows = [
    ...(acquirerKeys ?? []).map((k) => ({ ...k, sourceLabel: t("source_acquirer") })),
    ...(issuerKeys ?? []).map((k) => ({ ...k, sourceLabel: t("source_issuer") })),
  ];

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="text-left text-xs text-muted">
          <th className="py-2 font-medium">{t("keyType")}</th>
          <th className="py-2 font-medium">{t("counterparty")}</th>
          <th className="py-2 font-medium">{t("kcv")}</th>
          <th className="py-2 font-medium">{t("status")}</th>
          <th className="py-2 font-medium">{t("lifetime")}</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row, i) => (
          <tr key={`${row.sourceLabel}-${row.keyType}-${i}`} className="border-t border-border">
            <td className="py-2">{row.keyType}</td>
            <td className="py-2 text-muted">{row.counterparty ?? row.sourceLabel}</td>
            <td className="py-2 font-mono">{row.kcv}</td>
            <td className="py-2">{t(`status_${row.status}`)}</td>
            <td className="py-2">
              <KeyLifetimeBar daysRemaining={row.daysRemaining} lifetimeDays={row.lifetimeDays} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
