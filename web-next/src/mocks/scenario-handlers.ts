import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";

type Overview = components["schemas"]["Overview"];
type Link = components["schemas"]["Link"];
type TransactionSummary = components["schemas"]["TransactionSummary"];
type SwitchStatus = components["schemas"]["SwitchStatus"];
type KeyInfo = components["schemas"]["KeyInfo"];

// Every value below mirrors the Overview design canvas (docs/01-prd.md §7.1), so
// `pnpm dev:mock` renders the screen as designed in both Easy and Expert mode.

const TPS_PER_SAMPLE = [
  12, 15, 14, 18, 22, 20, 19, 25, 28, 26, 31, 34, 30, 29, 33, 38, 35, 32, 30, 27, 29, 31, 28, 26,
];
const SAMPLE_INTERVAL_MS = 150_000;

function throughputSeries(): Overview["throughput"] {
  const end = Date.now();
  return TPS_PER_SAMPLE.map((tps, i) => ({
    at: new Date(end - (TPS_PER_SAMPLE.length - 1 - i) * SAMPLE_INTERVAL_MS).toISOString(),
    tps,
  }));
}

const OVERVIEW: Omit<Overview, "throughput"> = {
  transactionsToday: 1950,
  transactionsDeltaPct: 0.12,
  approvalRate: 0.942,
  p99LatencyMs: 212,
  p50LatencyMs: 96,
  ledgerMatches: true,
  declineReasons: [
    { responseCode: "51", label: "Không đủ tiền", share: 0.41 },
    { responseCode: "55", label: "Sai mã PIN", share: 0.23 },
    { responseCode: "61", label: "Vượt hạn mức", share: 0.18 },
    { responseCode: "62", label: "Thẻ bị khóa", share: 0.12 },
    { responseCode: "", label: "Lý do khác", share: 0.06 },
  ],
};

const ECHO_AGE_MS = 12_000;

function links(): Link[] {
  return [
    {
      linkId: "issuer-primary",
      from: "acquirer",
      to: "issuer",
      status: "SIGNED_ON",
      lastEchoOk: true,
      lastEchoAt: new Date(Date.now() - ECHO_AGE_MS).toISOString(),
      p99LatencyMs: 212,
      inFlight: 0,
    },
  ];
}

const SWITCH: SwitchStatus = {
  circuit: "CLOSED",
  stipActive: false,
  stipLimit: { amount: 500_000, currency: "704" },
  stipApprovedCount: 0,
};

const ACQUIRER_KEYS: KeyInfo[] = [
  { keyType: "ZPK", counterparty: "issuer", kcv: "3F9A21", status: "ACTIVE", daysRemaining: 26, lifetimeDays: 365 },
  { keyType: "ZAK", counterparty: "issuer", kcv: "7C0E45", status: "ACTIVE", daysRemaining: 41, lifetimeDays: 365 },
];

// Rows mirror the canvas, limited to outcomes the real stack produces from
// contracts/fixtures/cards.json: 4417/5540 approve, 9021 (80 000 ₫) declines 51 above its balance,
// 1208 declines 61 above its 500 000 ₫ limit, 3310 is blocked (62), 7765 is expired (54). Only the
// last four digits appear here; the frontend never holds a PAN (web-next/CLAUDE.md).
// Deviations from the canvas: its ••••3310 approval and ••••7765 "Đã tự hủy · RC 91" cannot happen
// with those cards, so they use ••••5540, and a reversal after a timeout carries no RC. The canvas's
// "Sai mã PIN" (RC 55) needs the PIN forwarded to the issuer first (risk R-12).
type FeedRow = [string, string, string, number, TransactionSummary["status"], string | null, string];
const FEED: readonly FeedRow[] = [
  ["Cà phê Góc Phố", "00000042", "4417", 250_000, "APPROVED", "00", "Đã duyệt"],
  ["Nhà sách Ánh Dương", "00000043", "9021", 1_240_000, "DECLINED", "51", "Không đủ tiền"],
  ["Siêu thị Hoa Sen", "00000044", "5540", 486_500, "APPROVED", "00", "Đã duyệt"],
  ["Trạm xăng Bến Nghé", "00000045", "5540", 600_000, "REVERSED", null, "Đã tự hủy"],
  ["Quán bún Cô Ba", "00000046", "4417", 65_000, "APPROVED", "00", "Đã duyệt"],
  ["Tiệm bánh Mây", "00000047", "1208", 120_000, "DECLINED", "55", "Sai mã PIN"],
  ["Nhà thuốc Bình An", "00000048", "5540", 358_000, "SENT", null, "Đang chờ"],
];
const FEED_GAP_MS = 17_000;

function recentTransactions(): TransactionSummary[] {
  const now = Date.now();
  return FEED.map(([merchantName, terminalId, last4, amount, status, responseCode, responseLabel], i) => ({
    rrn: String(626_514_000_123 - i),
    stan: String(123 - i).padStart(6, "0"),
    type: "PURCHASE",
    status,
    responseCode,
    responseLabel,
    amount: { amount, currency: "704" },
    maskedPan: `970436******${last4}`,
    terminalId,
    merchantName,
    latencyMs: 96 + i * 11,
    createdAt: new Date(now - i * FEED_GAP_MS).toISOString(),
  }));
}

export const scenarioHandlers = [
  http.get("*/v1/transactions", () => HttpResponse.json({ items: recentTransactions(), nextCursor: null })),
  http.get("*/v1/metrics/overview", () =>
    HttpResponse.json({ ...OVERVIEW, throughput: throughputSeries() } satisfies Overview),
  ),
  http.get("*/v1/network/links", () => HttpResponse.json(links())),
  http.get("*/v1/network/saf", () => HttpResponse.json({ depth: 0, deadCount: 0, items: [] })),
  http.get("*/v1/network/switch", () => HttpResponse.json(SWITCH)),
  http.get("*/v1/keys/acquirer", () => HttpResponse.json(ACQUIRER_KEYS)),
];
