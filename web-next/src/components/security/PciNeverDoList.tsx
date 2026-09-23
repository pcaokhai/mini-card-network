"use client";

import { useTranslations } from "next-intl";
import type { DisplayMode } from "@/shared/state/display-mode";

const NEVER_DO_ITEM_KEYS = ["logPan", "logKey", "persistPinBlock", "panInUrl"] as const;

export function PciNeverDoList({ mode }: { mode: DisplayMode }) {
  const t = useTranslations("security.pciNeverDo");

  return (
    <section aria-labelledby="pci-never-do-heading" className="space-y-2">
      <h3 id="pci-never-do-heading" className="text-sm font-semibold">
        {t("heading")}
      </h3>
      <ul className="list-inside list-disc space-y-1 text-sm">
        {NEVER_DO_ITEM_KEYS.map((key) => (
          <li key={key}>{t(`${key}.${mode}`)}</li>
        ))}
      </ul>
    </section>
  );
}
