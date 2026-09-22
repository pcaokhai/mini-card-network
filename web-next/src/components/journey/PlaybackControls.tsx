import { useTranslations } from "next-intl";

interface PlaybackControlsProps {
  currentStep: number;
  totalSteps: number;
  isPlaying: boolean;
  onStepChange: (step: number) => void;
  onTogglePlay: () => void;
  onRestart: () => void;
}

export function PlaybackControls({
  currentStep,
  totalSteps,
  isPlaying,
  onStepChange,
  onTogglePlay,
  onRestart,
}: PlaybackControlsProps) {
  const t = useTranslations("journey.controls");
  const atStart = currentStep <= 0;
  const atEnd = currentStep >= totalSteps - 1;

  return (
    <div className="playback-controls flex items-center gap-2">
      <button type="button" data-testid="journey-restart" onClick={onRestart}>
        {t("restart")}
      </button>
      <button
        type="button"
        data-testid="journey-back"
        onClick={() => onStepChange(currentStep - 1)}
        disabled={atStart}
      >
        {t("back")}
      </button>
      <button type="button" data-testid="journey-toggle-play" onClick={onTogglePlay}>
        {isPlaying ? t("pause") : t("play")}
      </button>
      <button
        type="button"
        data-testid="journey-next"
        onClick={() => onStepChange(currentStep + 1)}
        disabled={atEnd}
      >
        {t("next")}
      </button>
    </div>
  );
}
