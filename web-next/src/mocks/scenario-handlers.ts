import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";

type Overview = components["schemas"]["Overview"];
type Link = components["schemas"]["Link"];
type TransactionSummary = components["schemas"]["TransactionSummary"];

const BAR_HEIGHTS = [
  38, 42, 44, 50, 56, 54, 52, 64, 68, 66, 74, 80, 76, 74, 84, 92, 86, 82, 80, 72, 78, 82, 76, 70,
];

const SAMPLE_INTERVAL_MS = 150_000;

function throughputSeries(): Overview["throughput"] {
  const end = Date.now();
  return BAR_HEIGHTS.map((tps, i) => ({
    at: new Date(end - (BAR_HEIGHTS.length - 1 - i) * SAMPLE_INTERVAL_MS).toISOString(),
    tps,
  }));
}

/** Mirrors the numbers on the Overview design canvas (docs/01-prd.md §7.1). */
const OVERVIEW: Omit<Overview, "throughput"> = {
  transactionsToday: 1950,
  transactionsDeltaPct: 0.12,
  approvalRate: 0.942,
  p99LatencyMs: 212,
  ledgerMatches: true,
  declineReasons: [
    { responseCode: "51", label: "Không đủ tiền", share: 0.41 },
    { responseCode: "55", label: "Sai mã PIN", share: 0.23 },
    { responseCode: "61", label: "Vượt hạn mức", share: 0.18 },
    { responseCode: "62", label: "Thẻ bị khóa", share: 0.12 },
    { responseCode: "96", label: "Lý do khác", share: 0.06 },
  ],
};

const LINKS: Link[] = [
  {
    linkId: "issuer-primary",
    from: "acquirer",
    to: "issuer",
    status: "SIGNED_ON",
    lastEchoOk: true,
    lastEchoAt: new Date(Date.now() - 12_000).toISOString(),
    p99LatencyMs: 212,
    inFlight: 0,
  },
];

// Test PANs come from contracts/fixtures/cards.json (BIN 970436); never a live card.
const FEED: readonly [string, string, string, number, TransactionSummary["status"], string][] = [
  ["Tiệm bánh Mây", "970436******4417", "00", 48_000, "APPROVED", "Đã duyệt"],
  ["Trạm xăng Bến Nghé", "970436******7765", "54", 500_000, "DECLINED", "Thẻ hết hạn"],
  ["Nhà sách Ánh Dương", "970436******1208", "00", 320_000, "APPROVED", "Đã duyệt"],
  ["Cà phê Góc Phố", "970436******5540", "00", 59_000, "APPROVED", "Đã duyệt"],
  ["Siêu thị Hoa Sen", "970436******9021", "51", 215_000, "DECLINED", "Không đủ tiền"],
  ["Tiệm bánh Mây", "970436******4417", "00", 48_000, "APPROVED", "Đã duyệt"],
  ["Trạm xăng Bến Nghé", "970436******7765", "54", 500_000, "DECLINED", "Thẻ hết hạn"],
];

const FEED_INTERVAL_MS = 4_000;

function recentTransactions(): TransactionSummary[] {
  const now = Date.now();
  return FEED.map(([merchantName, maskedPan, responseCode, amount, status, responseLabel], i) => ({
    rrn: String(240_921_000_000 + i),
    stan: String(100_000 + i).slice(-6),
    type: "PURCHASE",
    status,
    responseCode,
    responseLabel,
    amount: { amount, currency: "704" },
    maskedPan,
    terminalId: "TERM0001",
    merchantName,
    latencyMs: 180 + i * 7,
    createdAt: new Date(now - i * FEED_INTERVAL_MS).toISOString(),
  }));
}

export const scenarioHandlers = [
  http.get("*/v1/transactions", () =>
    HttpResponse.json({ items: recentTransactions(), nextCursor: null }),
  ),
  http.get("*/v1/metrics/overview", () =>
    HttpResponse.json({ ...OVERVIEW, throughput: throughputSeries() } satisfies Overview),
  ),
  http.get("*/v1/network/links", () => HttpResponse.json(LINKS)),
];
