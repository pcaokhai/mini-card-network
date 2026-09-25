import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";

export function ThroughputChart({ throughput }: { throughput: Overview["throughput"] }) {
  const t = useTranslations("overview.throughput");
  const maxTps = Math.max(1, ...throughput.map((sample) => sample.tps));
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
          style={{ transform: `scaleY(${sample.tps / maxTps})` }}
        />
      ))}
    </div>
  );
}
