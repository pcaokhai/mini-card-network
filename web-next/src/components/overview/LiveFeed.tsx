import { useTranslations } from "next-intl";
import Link from "next/link";
import { useRef, useState } from "react";
import { useWsEvents, type WsEnvelope } from "@/shared/ws/useWsEvents";
import { ThroughputChart } from "@/components/overview/ThroughputChart";
import { useRecentTransactions } from "@/shared/api/overview-client";
import { formatMoney } from "@/shared/format/money";
import type { components } from "@/shared/api/generated/schema";
import type { Overview } from "@/shared/api/overview-client";

type TransactionSummary = components["schemas"]["TransactionSummary"];

const MAX_ROWS = 30;
const BATCH_THRESHOLD_PER_SEC = 5;
const BATCH_WINDOW_MS = 500;

const EVENT_TYPES = ["transaction.created", "transaction.updated"];

const STATUS_CLASSES: Record<string, string> = {
  APPROVED: "bg-ok-soft text-ok",
  DECLINED: "bg-bad-soft text-bad",
  REVERSED: "bg-rev-soft text-rev",
  TIMEOUT: "bg-warn-soft text-warn",
  PENDING: "bg-canvas text-muted",
};

function last4(maskedPan: string | undefined): string {
  return maskedPan === undefined ? "" : maskedPan.slice(-4);
}

function timeOf(createdAt: string | undefined): string {
  if (createdAt === undefined) return "";
  const parsed = new Date(createdAt);
  return Number.isNaN(parsed.getTime()) ? "" : parsed.toLocaleTimeString("vi-VN", { hour12: false });
}

export function LiveFeed({
  expertMode = false,
  throughput = [],
}: {
  expertMode?: boolean;
  throughput?: Overview["throughput"];
}) {
  const t = useTranslations("overview.liveFeed");
  const tChart = useTranslations("overview.throughput");
  const tOverview = useTranslations("overview");
  const [rows, setRows] = useState<TransactionSummary[]>([]);
  const { data: seeded } = useRecentTransactions();
  const recentTimestampsRef = useRef<number[]>([]);
  const pendingRef = useRef<TransactionSummary[]>([]);
  const flushScheduledRef = useRef(false);

  function appendRows(incoming: TransactionSummary[]) {
    setRows((current) => [...current, ...incoming].slice(-MAX_ROWS));
  }

  function scheduleFlush() {
    if (flushScheduledRef.current) return;
    flushScheduledRef.current = true;
    setTimeout(() => {
      flushScheduledRef.current = false;
      const queued = pendingRef.current;
      pendingRef.current = [];
      if (queued.length > 0) appendRows(queued);
    }, BATCH_WINDOW_MS);
  }

  function isHighRate(now: number): boolean {
    const cutoff = now - 1000;
    const recent = recentTimestampsRef.current.filter((t) => t > cutoff);
    recent.push(now);
    recentTimestampsRef.current = recent;
    return recent.length > BATCH_THRESHOLD_PER_SEC;
  }

  useWsEvents(EVENT_TYPES, (event: WsEnvelope) => {
    const summary = event.data as TransactionSummary;
    if (isHighRate(Date.now())) {
      pendingRef.current = [...pendingRef.current, summary];
      scheduleFlush();
    } else {
      appendRows([summary]);
    }
  });

  // WS events are the live source; the REST snapshot only fills the table until one arrives.
  const visibleRows = rows.length > 0 ? rows : (seeded ?? []);
  const rowClass = expertMode ? "feed-row feed-row--expert" : "feed-row";

  return (
    <section
      aria-label={t("heading")}
      className="flex min-h-0 flex-col gap-4 rounded-card border border-border bg-surface px-5.5 py-5"
    >
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-[17px] font-semibold">{t("heading")}</h2>
        <span className="flex items-center gap-2 text-[13px] font-medium text-ok">
          <span aria-hidden className="relative size-2 shrink-0">
            <span className="absolute inset-0 animate-ping rounded-full bg-current motion-reduce:animate-none" />
            <span className="absolute inset-0 rounded-full bg-current" />
          </span>
          {t("liveBadge")}
        </span>
      </div>

      <div className="flex flex-col gap-2">
        <p className="text-xs text-muted">{expertMode ? tOverview("throughputTech") : tChart("heading")}</p>
        <ThroughputChart throughput={throughput} />
      </div>

      <div className={`${rowClass} border-b border-[#EFEDE6] pb-2 text-xs text-muted`}>
        <span>{t("time")}</span>
        <span>{t("merchant")}</span>
        <span>{t("card")}</span>
        <span className="text-right">{t("amount")}</span>
        {expertMode && <span>{t("rrn")}</span>}
        <span>{t("result")}</span>
      </div>

      <ul aria-label={t("heading")} data-testid="live-feed-list" className="flex flex-col">
        {visibleRows.length === 0 && <li className="py-3 text-sm text-muted">{t("empty")}</li>}
        {visibleRows.map((row, index) => (
          <li
            key={`${row.rrn}-${index}`}
            className={`feed-row--flash ${rowClass} border-b border-[#F2F0EA] py-3 text-sm last:border-b-0`}
          >
            <span className="font-mono text-[13px] text-muted">{timeOf(row.createdAt)}</span>
            <span className="truncate font-medium">{row.merchantName}</span>
            <span className="font-mono text-[13px]">
              <span aria-hidden>•••• </span>
              {last4(row.maskedPan)}
            </span>
            <span className="text-right font-semibold tabular-nums">
              {row.amount === undefined ? "" : formatMoney(row.amount)}
            </span>
            {/* Easy mode hides the RRN per the design canvas, but every row stays
                identifiable to screen readers and to the journey link. */}
            <Link
              href={`/transactions/${row.rrn}`}
              className={
                expertMode
                  ? "font-mono text-xs text-muted underline"
                  : "sr-only"
              }
            >
              {row.rrn}
            </Link>
            <span>
              <span
                className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ${
                  STATUS_CLASSES[row.status] ?? STATUS_CLASSES.PENDING
                }`}
              >
                <span aria-hidden className="size-1.5 rounded-full bg-current" />
                {row.responseLabel ?? row.status}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
