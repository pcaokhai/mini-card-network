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
    <section aria-labelledby="security-heading" className="space-y-6">
      <h1 id="security-heading" className="text-2xl font-bold">
        {t("title")}
      </h1>
      <section aria-labelledby="security-keys-heading" className="rounded-card border border-border bg-surface p-5">
        <h2 id="security-keys-heading" className="mb-3 text-base font-semibold">
          {t("keys.heading")}
        </h2>
        <KeyTable />
      </section>
      <section
        aria-labelledby="security-rotation-heading"
        className="rounded-card border border-border bg-surface p-5"
      >
        <h2 id="security-rotation-heading" className="mb-3 text-base font-semibold">
          {t("rotation.heading")}
        </h2>
        <div className="flex flex-wrap gap-4">
          {ROTATABLE_KEY_TYPES.map((keyType) => (
            <RotationStepper key={keyType} keyType={keyType} />
          ))}
        </div>
      </section>
      <div className="rounded-card border border-border bg-surface p-5">
        <PinBlockVisualiser />
      </div>
      <div className="rounded-card border border-border bg-surface p-5">
        <PciNeverDoList mode={mode} />
      </div>
    </section>
  );
}
