"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useRotation, useStartRotation, type RotatableKeyType } from "@/shared/api/security-client";

const STEP_STATUS_CLASSES: Record<string, string> = {
  PENDING: "bg-canvas text-muted",
  DONE: "bg-ok-soft text-ok",
  FAILED: "bg-bad-soft text-bad",
};

export function RotationStepper({ keyType }: { keyType: RotatableKeyType }) {
  const t = useTranslations("security.rotation");
  const [rotationId, setRotationId] = useState<string | null>(null);
  const startRotation = useStartRotation();
  const { data: rotation } = useRotation(rotationId);

  function handleRotate() {
    startRotation.mutate(keyType, {
      onSuccess: (data) => setRotationId(data.rotationId),
    });
  }

  return (
    <div className="space-y-3">
      <button
        type="button"
        onClick={handleRotate}
        disabled={startRotation.isPending}
        className="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-white disabled:opacity-50"
      >
        {startRotation.isPending ? t("rotating") : `${t("rotate")} (${keyType})`}
      </button>
      {rotation !== undefined && (
        <ol className="flex flex-wrap gap-2">
          {rotation.steps.map((step, i) => (
            <li
              key={`${step.name}-${i}`}
              className={`rounded-full px-3 py-1 text-xs font-semibold ${STEP_STATUS_CLASSES[step.status]}`}
            >
              {step.name} — {t(`stepStatus_${step.status}`)}
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
