"use client";

import { useTranslations } from "next-intl";

const PCI_ITEMS = ["panStorage", "cvv", "pinBlock", "keys", "logs", "audit"] as const;

/** PCI DSS rules the system keeps, in plain words (Easy) or as the mechanism that enforces them (Expert). */
export function PciNeverDoList({ expert }: { expert: boolean }) {
  const t = useTranslations("security.pci");

  return (
    <section aria-label={t("region")} className="security-panel security-pci">
      <h2 className="security-panel__heading">{t("heading")}</h2>
      <ul className="security-pci__list">
        {PCI_ITEMS.map((item) => (
          <li key={item} className="security-pci__item">
            <span className="security-pci__check" aria-hidden="true">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 12l5 5 9-10" />
              </svg>
            </span>
            <span>{t(`items.${item}.${expert ? "expert" : "easy"}`)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
