import { useTranslations } from "next-intl";
import "./journey.css";

interface PlaybackControlsProps {
  currentStep: number;
  totalSteps: number;
  isPlaying: boolean;
  onStepChange: (step: number) => void;
  onPlay: () => void;
  onRestart: () => void;
}

/** The canvas's step counter and Tự phát / Xem từ đầu / Bước trước / Bước tiếp buttons. */
export function PlaybackControls({ currentStep, totalSteps, isPlaying, onStepChange, onPlay, onRestart }: PlaybackControlsProps) {
  const t = useTranslations("journey");
  return (
    <div className="flex flex-wrap items-center gap-2 min-[1400px]:flex-nowrap">
      <span className="mr-1 text-[13px] text-muted" aria-live="polite">
        {t("steps.counter", { current: currentStep + 1, total: totalSteps })}
      </span>
      <button type="button" className="journey-button" data-variant="play" data-testid="journey-toggle-play" onClick={onPlay}>
        {isPlaying ? t("controls.playing") : t("controls.play")}
      </button>
      <button type="button" className="journey-button" data-testid="journey-restart" onClick={onRestart}>
        {t("controls.restart")}
      </button>
      <button
        type="button"
        className="journey-button"
        data-testid="journey-back"
        onClick={() => onStepChange(currentStep - 1)}
        disabled={currentStep <= 0}
      >
        {t("controls.back")}
      </button>
      <button
        type="button"
        className="journey-button"
        data-variant="primary"
        data-testid="journey-next"
        onClick={() => onStepChange(currentStep + 1)}
        disabled={currentStep >= totalSteps - 1}
      >
        {t("controls.next")}
      </button>
    </div>
  );
}
