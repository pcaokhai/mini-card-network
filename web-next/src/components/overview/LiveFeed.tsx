import { useTranslations } from "next-intl";
import Link from "next/link";
import { useRef, useState } from "react";
import { useWsEvents, type WsEnvelope } from "@/shared/ws/useWsEvents";
import { ThroughputChart } from "@/components/overview/ThroughputChart";
import { useRecentTransactions } from "@/shared/api/overview-client";
import { formatMoney } from "@/shared/format/money";
import { useResultLabel } from "@/shared/i18n/useResultLabel";
import { stagger } from "@/shared/motion/tokens";
import { PulseDot } from "@/shared/ui/PulseDot";
import { Term } from "@/shared/ui/Term";
import type { components } from "@/shared/api/generated/schema";
import type { Overview } from "@/shared/api/overview-client";

type TransactionSummary = components["schemas"]["TransactionSummary"];

const MAX_ROWS = 30;
const BATCH_THRESHOLD_PER_SEC = 5;
const BATCH_WINDOW_MS = 500;
const STAGGER_MS = stagger.step * 1000;

const EVENT_TYPES = ["transaction.created", "transaction.updated"];

const TONE_CLASSES = {
  ok: "bg-ok-soft text-ok",
  bad: "bg-bad-soft text-bad",
  warn: "bg-warn-soft text-warn",
  rev: "bg-rev-soft text-rev",
} as const;

const STATUS_TONE: Record<TransactionSummary["status"], keyof typeof TONE_CLASSES> = {
  CREATED: "warn",
  SENT: "warn",
  TIMED_OUT: "warn",
  REVERSAL_PENDING: "warn",
  APPROVED: "ok",
  DECLINED: "bad",
  FAILED: "bad",
  REVERSED: "rev",
};

function last4(maskedPan: string | undefined): string {
  return maskedPan === undefined ? "" : maskedPan.slice(-4);
}

function timeOf(createdAt: string | undefined): string {
  if (createdAt === undefined) return "";
  const parsed = new Date(createdAt);
  return Number.isNaN(parsed.getTime()) ? "" : parsed.toLocaleTimeString("vi-VN", { hour12: false });
}

/** Newest first, one row per RRN: a transaction.updated event replaces the row it updates. */
function mergeNewestFirst(incoming: TransactionSummary[], current: TransactionSummary[]): TransactionSummary[] {
  const newestFirst = [...incoming].reverse();
  const seen = new Set(newestFirst.map((row) => row.rrn));
  return [...newestFirst, ...current.filter((row) => !seen.has(row.rrn))].slice(0, MAX_ROWS);
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
  const label = useResultLabel();
  const [liveRows, setLiveRows] = useState<TransactionSummary[]>([]);
  // RRNs that arrived over the WebSocket; only these get the slide-in highlight.
  const [arrived, setArrived] = useState<ReadonlySet<string>>(new Set());
  const { data: seeded } = useRecentTransactions();
  const recentTimestampsRef = useRef<number[]>([]);
  const pendingRef = useRef<TransactionSummary[]>([]);
  const flushScheduledRef = useRef(false);

  function appendRows(incoming: TransactionSummary[]) {
    setLiveRows((current) => mergeNewestFirst(incoming, current));
    setArrived((current) => new Set([...current, ...incoming.map((row) => row.rrn)]));
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

  // The REST snapshot sits under the live rows so the first event adds to the table instead of replacing it.
  const visibleRows = mergeNewestFirst([...liveRows].reverse(), seeded ?? []);
  const rowClass = expertMode ? "feed-row feed-row--expert" : "feed-row";

  return (
    <section
      aria-label={t("heading")}
      className="flex min-h-0 flex-col gap-4 rounded-card border border-border bg-surface px-5.5 py-5"
    >
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-[17px] font-semibold">{t("heading")}</h2>
        <span className="flex items-center gap-2 text-[13px] font-medium text-ok">
          <PulseDot />
          {t("liveBadge")}
        </span>
      </div>

      <div className="flex flex-col gap-2">
        <p className="text-xs text-muted">{expertMode ? tOverview("throughputTech") : tChart("heading")}</p>
        <ThroughputChart throughput={throughput} />
      </div>

      <div className={`${rowClass} whitespace-nowrap border-b border-border pb-1 text-xs font-semibold text-[#6B6D75]`}>
        <span>{t("time")}</span>
        <span>{t("merchant")}</span>
        <span>{t("card")}</span>
        <span className="text-right">{t("amount")}</span>
        {expertMode && (
          <span>
            <Term id="rrn" />
          </span>
        )}
        <span>{t("result")}</span>
      </div>

      <ul aria-label={t("heading")} data-testid="live-feed-list" aria-live="polite" className="flex flex-col gap-4">
        {visibleRows.length === 0 && <li className="text-sm text-muted">{t("empty")}</li>}
        {visibleRows.map((row, index) => {
          const isNew = arrived.has(row.rrn);
          const result = label.forTransaction(row.status, row.responseCode, row.responseLabel);
          return (
            <li
              key={row.rrn}
              data-new={isNew || undefined}
              className={`${rowClass} relative border-b border-[#F0EEE8] py-2.5 text-sm last:border-b-0 ${
                isNew ? "animate-mcn-row-in" : "animate-mcn-fade-up"
              }`}
              style={isNew ? undefined : { animationDelay: `${Math.min(index, stagger.maxItems) * STAGGER_MS}ms` }}
            >
              {isNew && (
                <span
                  aria-hidden
                  className="pointer-events-none absolute -inset-x-2 inset-y-0 animate-mcn-row-flash rounded-md bg-[#FFF3D6]"
                />
              )}
              <span className="relative font-mono text-[13px] text-muted">{timeOf(row.createdAt)}</span>
              <span className="relative truncate font-medium">{row.merchantName}</span>
              <span className="relative font-mono text-[13px]">
                <span aria-hidden>•••• </span>
                {last4(row.maskedPan)}
              </span>
              <span className="relative text-right font-semibold tabular-nums">
                {row.amount === undefined ? "" : formatMoney(row.amount)}
              </span>
              {/* Easy mode hides the RRN per the design canvas, but every row stays
                  identifiable to screen readers and to the journey link. */}
              <Link
                href={`/transactions/${row.rrn}`}
                className={expertMode ? "relative font-mono text-xs text-muted hover:underline" : "sr-only"}
              >
                {row.rrn}
              </Link>
              <span className="relative">
                <span
                  data-testid="feed-status"
                  className={`inline-flex h-6.5 items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 text-xs font-semibold ${
                    TONE_CLASSES[STATUS_TONE[row.status] ?? "warn"]
                  }`}
                >
                  <span aria-hidden className="size-1.5 rounded-full bg-current" />
                  {/* MCN-306-AC3: Expert mode shows the ISO 8583 response code beside the result. */}
                  {expertMode && row.responseCode ? `${result} · RC ${row.responseCode}` : result}
                </span>
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
