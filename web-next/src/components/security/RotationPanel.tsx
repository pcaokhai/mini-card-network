"use client";

import { useTranslations } from "next-intl";
import type { KeyRotation } from "@/shared/api/security-client";
import { rotationSteps } from "./security-model";
import type { RotationPhase } from "./useZpkRotation";

type RotationPanelProps = {
  rotation: KeyRotation | undefined;
  phase: RotationPhase;
  startFailed: boolean;
  /** The rotation started but its resource can no longer be read. */
  lostTrack: boolean;
  expert: boolean;
  onStart: () => void;
  onReset: () => void;
};

const PENDING_KCV = "······";

/** The four steps of a ZPK rotation, each lit as the rotation resource reports it done. */
export function RotationPanel({ rotation, phase, startFailed, lostTrack, expert, onStart, onReset }: RotationPanelProps) {
  const t = useTranslations("security.rotation");
  const kcv = rotation?.newKcv ?? PENDING_KCV;
  const label = phase === "running" ? t("running") : phase === "completed" ? t("done") : t("start");

  return (
    <section aria-label={t("region")} className="security-panel security-rotation">
      <h2 className="security-panel__heading">{t("heading")}</h2>
      <p className="security-intro">{t("intro")}</p>
      <ol className="security-steps">
        {rotationSteps(rotation).map(({ name, status }) => (
          <li key={name} className="security-step" data-status={status}>
            <span className="security-step__dot" aria-hidden="true" />
            <span className="security-step__text">
              <span className="security-step__title">
                {t(`steps.${name}.title`)}
                <span className="sr-only"> · {t(`stepStatus.${status}`)}</span>
              </span>
              <span className="security-step__detail">
                {status === "FAILED" ? t("stepFailed") : expert ? t(`steps.${name}.expert`, { kcv }) : t(`steps.${name}.easy`)}
              </span>
            </span>
          </li>
        ))}
      </ol>
      {(startFailed || lostTrack) && (
        <p role="alert" className="security-error">
          {t(startFailed ? "failed" : "lost")}
        </p>
      )}
      <div className="security-rotation__actions">
        <button
          type="button"
          className="security-button"
          disabled={phase === "running"}
          onClick={phase === "completed" ? onReset : onStart}
        >
          {label}
        </button>
      </div>
    </section>
  );
}
