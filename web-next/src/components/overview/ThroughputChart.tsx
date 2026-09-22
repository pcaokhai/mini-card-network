import { useTranslations } from "next-intl";
import type { Overview } from "@/shared/api/overview-client";

export function ThroughputChart({ throughput }: { throughput: Overview["throughput"] }) {
  const t = useTranslations("overview.throughput");
  const maxTps = Math.max(1, ...throughput.map((sample) => sample.tps));

  return (
    <div>
      <h2 className="mb-2 text-lg font-semibold">{t("heading")}</h2>
      <div
        className="throughput-chart"
        role="img"
        aria-label={`${t("heading")}: ${throughput.map((s) => s.tps).join(", ")}`}
      >
        {throughput.map((sample, index) => (
          <div
            key={sample.at + index}
            className="throughput-bar"
            style={{ transform: `scaleY(${sample.tps / maxTps})` }}
          />
        ))}
      </div>
    </div>
  );
}
