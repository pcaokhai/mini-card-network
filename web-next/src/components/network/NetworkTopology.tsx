import { Fragment } from "react";
import { useTranslations } from "next-intl";
import { buildTopology, type Segment, type TopologyNode } from "./network-model";
import type { Link, SwitchStatus } from "@/shared/api/network-client";

interface NetworkTopologyProps {
  links: Link[];
  switchStatus: SwitchStatus | undefined;
  terminalCount: number | undefined;
  expert: boolean;
}

function useNodeSub(expert: boolean) {
  const t = useTranslations("network.topology");
  return (node: TopologyNode, circuit: string): string => {
    const pick = (easy: string, tech: string) => t(`${node.id}.${expert ? tech : easy}`, { count: node.count, names: node.instances.join(", "), circuit });
    switch (node.id) {
      case "pos":
        return node.tone === "unavailable" ? pick("easyUnavailable", "techUnavailable") : pick("easy", "tech");
      case "switch":
        if (node.tone === "unavailable") return pick("easyUnavailable", "techUnavailable");
        return node.tone === "warn" ? pick("easyStip", "techStip") : pick("easy", "tech");
      default:
        return node.tone === "bad" ? pick("easyDown", "techDown") : pick("easy", "tech");
    }
  };
}

function SegmentLine({ state, label }: { state: Segment; label: string }) {
  return (
    <li className="net-seg" data-state={state}>
      <span className="sr-only">{label}</span>
      <span aria-hidden className="net-seg__line" />
      <svg aria-hidden width="12" height="12" viewBox="0 0 12 12">
        <path d="M2 1l6 5-6 5" fill="none" stroke="currentColor" strokeWidth="2" />
      </svg>
    </li>
  );
}

/** MCN-205-AC1 / MCN-804-AC1: POS → acquirer → switch → issuer, links flowing in the request direction. */
export function NetworkTopology({ links, switchStatus, terminalCount, expert }: NetworkTopologyProps) {
  const t = useTranslations("network.topology");
  const nodeSub = useNodeSub(expert);
  const { nodes, segments } = buildTopology({ links, switchStatus, terminalCount });
  const circuit = switchStatus?.circuit ?? "";
  const titles = nodes.map((node) => t(`${node.id}.title`));

  return (
    <section aria-label={t("ariaLabel")} className="net-card gap-3.5">
      <h2>{t("heading")}</h2>
      <ol className="net-topo">
        {nodes.map((node, i) => {
          const segment = segments[i - 1];
          return (
            <Fragment key={node.id}>
              {segment !== undefined && (
                <SegmentLine
                  state={segment}
                  label={t(segment === "up" ? "segmentUp" : "segmentDown", { from: titles[i - 1] ?? "", to: titles[i] ?? "" })}
                />
              )}
              <li className="net-node" data-tone={node.tone} data-testid={`topo-${node.id}`}>
                <span className="net-node__title">
                  <span aria-hidden className="net-dot" />
                  {titles[i]}
                </span>
                <span className="net-node__sub">{nodeSub(node, circuit)}</span>
              </li>
            </Fragment>
          );
        })}
      </ol>
    </section>
  );
}
