import { delay, http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";

type Overview = components["schemas"]["Overview"];
type Link = components["schemas"]["Link"];
type TransactionSummary = components["schemas"]["TransactionSummary"];
type SwitchStatus = components["schemas"]["SwitchStatus"];
type KeyInfo = components["schemas"]["KeyInfo"];
type Transaction = components["schemas"]["Transaction"];

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

// POS outcomes per fixture card (contracts/fixtures/cards.json), matching the real issuer: last four
// digits and balances only, never a PAN. Balances live in memory so a payment shows on the tiles.
const POS_CARDS: Record<string, { cardRef: string; last4: string; decline?: string; limit?: number }> = {
  tok_normal: { cardRef: "crd_normal0001", last4: "4417" },
  tok_low: { cardRef: "crd_lowbal0002", last4: "9021" },
  tok_blocked: { cardRef: "crd_blockd0003", last4: "3310", decline: "62" },
  tok_expired: { cardRef: "crd_expird0004", last4: "7765", decline: "54" },
  tok_limit: { cardRef: "crd_limit00005", last4: "1208", limit: 500_000 },
  tok_second: { cardRef: "crd_second0006", last4: "5540" },
};
const posBalances = new Map([
  ["crd_normal0001", 5_000_000],
  ["crd_lowbal0002", 80_000],
  ["crd_blockd0003", 2_000_000],
  ["crd_expird0004", 1_500_000],
  ["crd_limit00005", 50_000_000],
  ["crd_second0006", 200_000_000],
]);
const heldByRrn = new Map<string, { cardToken: string; amount: number }>();
let posStan = 124;
const POS_LATENCY_MS = 1_100; // the canvas's processing time, so the spinner and dots are visible

type PosRequest = { cardToken?: string; amount?: { amount: number } };

function posTransaction(type: Transaction["type"], cardToken: string, amount: number): Transaction {
  const card = POS_CARDS[cardToken] ?? { cardRef: "crd_normal0001", last4: "4417" };
  const available = posBalances.get(card.cardRef) ?? 0;
  const debits = type === "PURCHASE" || type === "PREAUTH";
  const rc =
    card.decline ??
    (card.limit !== undefined && amount > card.limit ? "61" : debits && amount > available ? "51" : "00");
  const stan = String(posStan++).padStart(6, "0");
  if (rc === "00" && type !== "BALANCE") {
    posBalances.set(card.cardRef, available + (type === "REFUND" ? amount : type === "COMPLETION" ? 0 : -amount));
  }
  return {
    rrn: `626807${stan}`,
    stan,
    type,
    status: rc === "00" ? "APPROVED" : "DECLINED",
    responseCode: rc,
    authCode: rc === "00" ? `A${stan.slice(-5)}` : null,
    amount: { amount, currency: "704" },
    balance: type === "BALANCE" && rc === "00" ? { amount: available, currency: "704" } : null,
    maskedPan: `970436******${card.last4}`,
    terminalId: "00000042",
    merchantName: "Cà phê Góc Phố",
    createdAt: new Date().toISOString(),
  };
}

// Replays by Idempotency-Key like the gateway does. It also keeps a payment from being applied
// twice when React StrictMode starts two MSW clients in dev and both run the resolver.
const posReplies = new Map<string, Transaction>();

function once(request: Request, create: () => Transaction): Transaction {
  const key = request.headers.get("Idempotency-Key");
  const seen = key === null ? undefined : posReplies.get(key);
  if (seen) return seen;
  const tx = create();
  if (key !== null) posReplies.set(key, tx);
  return tx;
}

function posHandler(type: Transaction["type"]) {
  return async ({ request }: { request: Request }) => {
    const body = (await request.clone().json()) as PosRequest;
    const tx = once(request, () => posTransaction(type, body.cardToken ?? "tok_normal", body.amount?.amount ?? 0));
    if (type === "PREAUTH" && tx.status === "APPROVED") {
      heldByRrn.set(tx.rrn, { cardToken: body.cardToken ?? "tok_normal", amount: tx.amount.amount });
    }
    await delay(POS_LATENCY_MS);
    return HttpResponse.json(tx, { status: 201 });
  };
}

const posHandlers = [
  http.get("*/v1/cards/:cardRef", ({ params }) => {
    const amount = posBalances.get(String(params.cardRef));
    if (amount === undefined) return new HttpResponse(null, { status: 404 });
    const money = { amount, currency: "704" };
    return HttpResponse.json({ cardRef: params.cardRef, availableBalance: money, ledgerBalance: money });
  }),
  http.post("*/v1/transactions/purchases", posHandler("PURCHASE")),
  http.post("*/v1/transactions/pre-authorizations", posHandler("PREAUTH")),
  http.post("*/v1/transactions/refunds", posHandler("REFUND")),
  http.post("*/v1/transactions/balance-inquiries", posHandler("BALANCE")),
  http.post("*/v1/transactions/:rrn/completions", async ({ params, request }) => {
    const { amount } = (await request.clone().json()) as PosRequest;
    const held = heldByRrn.get(String(params.rrn));
    const tx = once(request, () => {
      const created = posTransaction("COMPLETION", held?.cardToken ?? "tok_normal", amount?.amount ?? 0);
      const found = held ? created : { ...created, status: "DECLINED" as const, responseCode: "25", authCode: null };
      return { ...found, originalRrn: String(params.rrn) };
    });
    await delay(POS_LATENCY_MS);
    return HttpResponse.json(tx, { status: 201 });
  }),
];

export const scenarioHandlers = [
  http.get("*/v1/transactions", () => HttpResponse.json({ items: recentTransactions(), nextCursor: null })),
  http.get("*/v1/metrics/overview", () =>
    HttpResponse.json({ ...OVERVIEW, throughput: throughputSeries() } satisfies Overview),
  ),
  http.get("*/v1/network/links", () => HttpResponse.json(links())),
  http.get("*/v1/network/saf", () => HttpResponse.json({ depth: 0, deadCount: 0, items: [] })),
  http.get("*/v1/network/switch", () => HttpResponse.json(SWITCH)),
  http.get("*/v1/keys/acquirer", () => HttpResponse.json(ACQUIRER_KEYS)),
  ...posHandlers,
];
