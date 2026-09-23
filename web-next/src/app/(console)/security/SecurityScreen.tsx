"use client";

import { useTranslations } from "next-intl";
import { useDisplayMode } from "@/shared/state/display-mode";
import { KeyTable } from "@/components/security/KeyTable";
import { RotationStepper } from "@/components/security/RotationStepper";
import { PinBlockVisualiser } from "@/components/security/PinBlockVisualiser";
import { PciNeverDoList } from "@/components/security/PciNeverDoList";
import "@/components/security/security.css";

const ROTATABLE_KEY_TYPES = ["ZPK", "ZAK"] as const;

export function SecurityScreen() {
  const t = useTranslations("security");
  const mode = useDisplayMode((s) => s.mode);

  return (
    <section aria-labelledby="security-heading" className="space-y-8">
      <h1 id="security-heading" className="text-2xl font-bold">
        {t("title")}
      </h1>
      <section aria-labelledby="security-keys-heading" className="space-y-4">
        <h2 id="security-keys-heading" className="text-lg font-semibold">
          {t("keys.heading")}
        </h2>
        <KeyTable />
        <div className="flex flex-wrap gap-4">
          {ROTATABLE_KEY_TYPES.map((keyType) => (
            <RotationStepper key={keyType} keyType={keyType} />
          ))}
        </div>
      </section>
      <PinBlockVisualiser />
      <PciNeverDoList mode={mode} />
    </section>
  );
}
