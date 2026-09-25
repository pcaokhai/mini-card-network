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
  { keyType: "ZPK", counterparty: "issuer", kcv: "3F9A21", status: "ACTIVE", daysRemaining: 26, lifetimeDays: 90 },
  { keyType: "ZAK", counterparty: "issuer", kcv: "7C0E45", status: "ACTIVE", daysRemaining: 41, lifetimeDays: 90 },
];

// Test PANs only (contracts/fixtures/cards.json, BIN 970436). The canvas also shows
// ••••1208 and ••••5540, which are not fixture cards, so those rows reuse fixture cards.
type FeedRow = [string, string, number, TransactionSummary["status"], string | null, string];
const FEED: readonly FeedRow[] = [
  ["Cà phê Góc Phố", "9704360000004417", 250_000, "APPROVED", "00", "Đã duyệt"],
  ["Nhà sách Ánh Dương", "9704360000009021", 1_240_000, "DECLINED", "51", "Không đủ tiền"],
  ["Siêu thị Hoa Sen", "9704360000003310", 486_500, "APPROVED", "00", "Đã duyệt"],
  ["Trạm xăng Bến Nghé", "9704360000007765", 600_000, "REVERSED", "91", "Đã tự hủy"],
  ["Quán bún Cô Ba", "9704360000004417", 65_000, "APPROVED", "00", "Đã duyệt"],
  ["Tiệm bánh Mây", "9704360000003310", 120_000, "DECLINED", "55", "Sai mã PIN"],
  ["Nhà thuốc Bình An", "9704360000009021", 358_000, "SENT", null, "Đang chờ"],
];
const FEED_GAP_MS = 17_000;

function maskPan(pan: string): string {
  return `${pan.slice(0, 6)}******${pan.slice(-4)}`;
}

function recentTransactions(): TransactionSummary[] {
  const now = Date.now();
  return FEED.map(([merchantName, pan, amount, status, responseCode, responseLabel], i) => ({
    rrn: String(626_514_000_123 - i),
    stan: String(123 - i).padStart(6, "0"),
    type: "PURCHASE",
    status,
    responseCode,
    responseLabel,
    amount: { amount, currency: "704" },
    maskedPan: maskPan(pan),
    terminalId: "TERM0001",
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
