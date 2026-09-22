import { useTranslations } from "next-intl";
import "./journey.css";

const RADIUS = 18;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

interface CountdownRingProps {
  durationMs: number;
  isPlaying: boolean;
  labelKey?: string;
}

export function CountdownRing({ durationMs, isPlaying, labelKey = "journey.countdown.ariaLabel" }: CountdownRingProps) {
  const t = useTranslations();
  return (
    <svg
      data-testid="countdown-ring"
      data-playing={isPlaying}
      className="countdown-ring"
      width="44"
      height="44"
      viewBox="0 0 44 44"
      role="img"
      aria-label={t(labelKey)}
      style={{ animationDuration: `${durationMs}ms` }}
    >
      <circle cx="22" cy="22" r={RADIUS} className="countdown-ring-track" />
      <circle
        cx="22"
        cy="22"
        r={RADIUS}
        className="countdown-ring-progress"
        strokeDasharray={CIRCUMFERENCE}
        style={{ animationPlayState: isPlaying ? "running" : "paused", animationDuration: `${durationMs}ms` }}
      />
    </svg>
  );
}
