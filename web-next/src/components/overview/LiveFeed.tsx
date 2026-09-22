import { useTranslations } from "next-intl";
import Link from "next/link";
import { useRef, useState } from "react";
import { useWsEvents, type WsEnvelope } from "@/shared/ws/useWsEvents";
import type { components } from "@/shared/api/generated/schema";

type TransactionSummary = components["schemas"]["TransactionSummary"];

const MAX_ROWS = 30;
const BATCH_THRESHOLD_PER_SEC = 5;
const BATCH_WINDOW_MS = 500;

const EVENT_TYPES = ["transaction.created", "transaction.updated"];

export function LiveFeed({ expertMode = false }: { expertMode?: boolean }) {
  const t = useTranslations("overview.liveFeed");
  const [rows, setRows] = useState<TransactionSummary[]>([]);
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

  return (
    <div>
      <h2 className="mb-2 text-lg font-semibold">{t("heading")}</h2>
      <ul aria-label={t("heading")} data-testid="live-feed-list" className="space-y-1">
        {rows.length === 0 && <li className="text-sm text-muted">{t("empty")}</li>}
        {rows.map((row, index) => (
          <li key={`${row.rrn}-${index}`} className="feed-row--flash flex items-center gap-3 text-sm">
            <Link href={`/transactions/${row.rrn}`} className="font-mono text-xs text-muted underline">
              {row.rrn}
            </Link>
            <span>{row.merchantName}</span>
            <span>{row.status}</span>
            {expertMode && row.responseCode && (
              <span data-expert-only className="font-mono text-xs text-muted">
                {row.responseCode}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
