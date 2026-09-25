"use client";

import { useTranslations } from "next-intl";
import { KeyTable } from "@/components/security/KeyTable";
import { PciNeverDoList } from "@/components/security/PciNeverDoList";
import { PinBlockVisualiser } from "@/components/security/PinBlockVisualiser";
import { RotationPanel } from "@/components/security/RotationPanel";
import { useZpkRotation } from "@/components/security/useZpkRotation";
import { useAcquirerKeys } from "@/shared/api/security-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import "@/components/security/security.css";

export function SecurityScreen() {
  const t = useTranslations("security");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const keys = useAcquirerKeys();
  const rotation = useZpkRotation();
  const zpkKcv = keys.data?.find((k) => k.keyType === "ZPK" && k.status === "ACTIVE")?.kcv ?? "";
  const canRotate = rotation.phase === "idle" || rotation.phase === "failed";

  return (
    <section aria-labelledby="security-heading" className="security-root flex flex-col gap-5">
      <div>
        <h1 id="security-heading" className="text-[30px] font-bold tracking-[-0.01em]">
          {t("title")}
        </h1>
        <p className="mt-1.5 text-[15px] text-muted">{t("subtitle")}</p>
      </div>
      {keys.isError && (
        <p role="alert" className="security-error">
          {t("loadFailed")}
        </p>
      )}
      <KeyTable
        keys={keys.data ?? []}
        expert={expert}
        canRotate={canRotate}
        onRotate={rotation.start}
        newKcv={rotation.phase === "completed" ? (rotation.rotation?.newKcv ?? null) : null}
      />
      <div className="security-columns">
        <RotationPanel
          rotation={rotation.rotation}
          phase={rotation.phase}
          startFailed={rotation.startFailed}
          lostTrack={rotation.lostTrack}
          expert={expert}
          onStart={rotation.start}
          onReset={rotation.reset}
        />
        <PinBlockVisualiser expert={expert} zpkKcv={zpkKcv} />
        <PciNeverDoList expert={expert} />
      </div>
    </section>
  );
}
