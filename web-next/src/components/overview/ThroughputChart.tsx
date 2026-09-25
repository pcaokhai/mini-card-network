import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";

// The canvas grows each bar 25 ms after the one before it.
const BAR_STAGGER_MS = 25;

export function ThroughputChart({ throughput }: { throughput: Overview["throughput"] }) {
  const t = useTranslations("overview.throughput");
  // Scaled to the busiest bucket, however small: a lab day runs well under 1 transaction/second.
  const maxTps = Math.max(0, ...throughput.map((sample) => sample.tps));
  const scale = (tps: number) => (maxTps > 0 ? tps / maxTps : 0);
  const lastIndex = throughput.length - 1;

  return (
    <div
      className="throughput-chart"
      role="img"
      aria-label={`${t("heading")}: ${throughput.map((s) => s.tps).join(", ")}`}
    >
      {throughput.map((sample, index) => (
        <div
          key={sample.at + index}
          className={index === lastIndex ? "throughput-bar throughput-bar--latest" : "throughput-bar"}
          style={{ transform: `scaleY(${scale(sample.tps)})` }}
        >
          <div className="throughput-bar__fill animate-mcn-grow" style={{ animationDelay: `${index * BAR_STAGGER_MS}ms` }} />
        </div>
      ))}
    </div>
  );
}
