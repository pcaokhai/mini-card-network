import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";
import { MOCK_CARDS, MOCK_LEDGERS } from "../journey-fixtures";

type CardDetail = components["schemas"]["CardDetail"];
type CardLimits = components["schemas"]["CardLimits"];
type JournalEntry = components["schemas"]["JournalEntry"];
type AuditEntry = components["schemas"]["AuditEntry"];
type Hold = CardDetail["holds"][number];

const vnd = (amount: number) => ({ amount, currency: "704" });

// The design canvas (Cards.dc.html) per card. The cardRefs, masked PANs and statuses stay those of
// MOCK_CARDS, which the POS and Journey screens read too; 1208 and 5540 are not in the canvas.
interface CanvasCard {
  holderName: string;
  ledgerBalance: number;
  usedToday: number;
  limits: [daily: number, perTransaction: number];
  holds: Hold[];
  journal: JournalEntry[];
}

const at = (hhmm: string) => `2026-09-21T${hhmm}:00+07:00`;
const leg = (account: string, direction: "DEBIT" | "CREDIT", amount: number) => ({ account, direction, amount: vnd(amount) });

/** A debit (customer out) or a credit back to the customer, posted as the issuer does; 0 posts nothing. */
function journal(journalId: string, time: string, description: string, entryType: JournalEntry["entryType"], cardRef: string, effect: number, rrn: string | null = null, from = "SETTLEMENT_SUSPENSE"): JournalEntry {
  const customer = `ACC-${cardRef}`;
  const postings =
    effect < 0 ? [leg(customer, "DEBIT", -effect), leg("SETTLEMENT_SUSPENSE", "CREDIT", -effect)]
    : effect > 0 ? [leg(from, "DEBIT", effect), leg(customer, "CREDIT", effect)]
    : [];
  return { journalId, occurredAt: at(time), description, entryType, rrn, postings };
}

const CANVAS: Record<string, CanvasCard> = {
  crd_normal0001: {
    holderName: "Nguyễn Minh Anh",
    ledgerBalance: 5_000_000,
    usedToday: 915_000,
    limits: [10_000_000, 5_000_000],
    holds: [{ holdId: "hold_4417_1", merchantName: "Khách sạn Hoa Biển", amount: vnd(1_500_000), expiresAt: "2026-09-26T12:00:00+07:00", status: "ACTIVE" }],
    journal: [
      // RRN of the Journey fixture's approved purchase, so its money panel still finds its journal.
      journal("3", "14:32", "Mua hàng · Cà phê Góc Phố", "PURCHASE", "crd_normal0001", -250_000, "626514000123"),
      journal("12", "14:31", "Mua hàng · Quán bún Cô Ba", "PURCHASE", "crd_normal0001", -65_000),
      journal("11", "14:31", "Hoàn tiền tự động · Trạm xăng Bến Nghé", "REVERSAL", "crd_normal0001", 600_000),
      journal("10", "14:31", "Mua hàng · Trạm xăng Bến Nghé", "PURCHASE", "crd_normal0001", -600_000),
      journal("9", "09:12", "Nhận chuyển khoản lương", "ADJUSTMENT", "crd_normal0001", 15_000_000, null, "INCOMING_TRANSFER"),
    ],
  },
  crd_lowbal0002: {
    holderName: "Trần Thu Hà",
    ledgerBalance: 80_000,
    usedToday: 986_500,
    limits: [5_000_000, 2_000_000],
    holds: [],
    journal: [
      // Declines post nothing; the canvas still lists them, so dev:mock serves them without postings.
      journal("22", "14:31", "Từ chối · Nhà sách Ánh Dương (không đủ tiền)", "PURCHASE", "crd_lowbal0002", 0),
      journal("21", "11:20", "Mua hàng · Siêu thị Hoa Sen", "PURCHASE", "crd_lowbal0002", -486_500),
      journal("20", "10:02", "Rút tiền mặt · ATM Bến Thành", "CASH", "crd_lowbal0002", -500_000),
    ],
  },
  crd_blockd0003: {
    holderName: "Lê Quốc Bảo",
    ledgerBalance: 2_000_000,
    usedToday: 0,
    limits: [5_000_000, 2_000_000],
    holds: [],
    journal: [journal("30", "08:40", "Chủ thẻ báo mất thẻ qua tổng đài", "ADJUSTMENT", "crd_blockd0003", 0)],
  },
  crd_expird0004: {
    holderName: "Phạm Gia Huy",
    ledgerBalance: 1_500_000,
    usedToday: 0,
    limits: [3_000_000, 2_000_000],
    holds: [],
    journal: [journal("40", "13:05", "Từ chối · Tiệm bánh Mây (thẻ hết hạn)", "PURCHASE", "crd_expird0004", 0)],
  },
};

// Holders of the two cards the canvas does not show, with the diacritics the canvas uses for the rest.
const HOLDERS: Record<string, string> = { crd_limit00005: "Võ Thanh Tâm", crd_second0006: "Đặng Ngọc Linh" };

interface CardState {
  status: CardDetail["status"];
  limits: CardLimits;
  version: number;
}

function initialState(): Map<string, CardState> {
  return new Map(
    MOCK_CARDS.map((card) => {
      const [daily, perTransaction] = CANVAS[card.cardRef]?.limits ?? [20_000_000, 500_000];
      return [card.cardRef, { status: card.status, limits: { dailyAmount: vnd(daily), perTransactionAmount: vnd(perTransaction), dailyCount: null }, version: 1 }];
    }),
  );
}

// ponytail: one in-memory store per page load (or per test file); a reload starts from the canvas again.
let state = initialState();

// Admin actions per card, newest first, as the issuer's audit_log returns them.
let audit = new Map<string, AuditEntry[]>();

/** Tests that block, unblock or change limits start from the canvas values again. */
export function resetCardsMock() {
  state = initialState();
  audit = new Map();
}

/** The BFF names the actor (X-Actor); straight MSW calls in dev:mock skip the BFF, so default it. */
function recordAudit(cardRef: string, request: Request, action: AuditEntry["action"]) {
  const entry: AuditEntry = {
    auditId: `a${Date.now()}${Math.random().toString(16).slice(2, 6)}`,
    occurredAt: new Date().toISOString(),
    actor: request.headers.get("X-Actor") ?? "console",
    action,
    before: null,
    after: null,
  };
  audit = new Map(audit).set(cardRef, [entry, ...(audit.get(cardRef) ?? [])]);
}

const etagOf = (cardRef: string) => `"v${state.get(cardRef)?.version ?? 0}"`;

function summary(card: (typeof MOCK_CARDS)[number]) {
  const { cardRef, maskedPan, holderName, expiry } = card;
  return { cardRef, maskedPan, holderName: CANVAS[cardRef]?.holderName ?? HOLDERS[cardRef] ?? holderName, status: state.get(cardRef)?.status ?? card.status, expiry };
}

/** Also seeds the Cards stories, so they render without a network. */
export function mockCardDetail(cardRef: string): CardDetail | null {
  const card = MOCK_CARDS.find((c) => c.cardRef === cardRef);
  const cardState = state.get(cardRef);
  if (!card || !cardState) return null;
  const canvas = CANVAS[cardRef];
  const ledger = canvas?.ledgerBalance ?? card.balance;
  const holds = canvas?.holds ?? [];
  const held = holds.filter((h) => h.status === "ACTIVE").reduce((sum, h) => sum + h.amount.amount, 0);
  return {
    ...summary(card),
    ledgerBalance: vnd(ledger),
    availableBalance: vnd(ledger - held),
    holds,
    limits: cardState.limits,
    usedToday: vnd(canvas?.usedToday ?? 0),
  };
}

function withCard(cardRef: string, next: (current: CardState) => CardState) {
  const current = state.get(cardRef);
  if (current) state = new Map(state).set(cardRef, next(current));
}

function detailResponse(cardRef: string) {
  const detail = mockCardDetail(cardRef);
  return detail ? HttpResponse.json(detail, { headers: { ETag: etagOf(cardRef) } }) : new HttpResponse(null, { status: 404 });
}

export const mockCardSummaries = () => MOCK_CARDS.map(summary);
export const mockLedger = (cardRef: string): JournalEntry[] => CANVAS[cardRef]?.journal ?? MOCK_LEDGERS[cardRef] ?? [];

const staleEtag = () =>
  HttpResponse.json(
    { type: "https://mcn.local/problems/precondition-failed", title: "ETag mismatch", status: 412, detail: "If-Match does not match the current ETag." },
    { status: 412, headers: { "Content-Type": "application/problem+json" } },
  );

/** Cards and Accounts (and the POS tiles and Journey balances): the Issuer Admin card API. */
export const cardsHandlers = [
  http.get("*/v1/cards", () => HttpResponse.json(mockCardSummaries())),
  http.get("*/v1/cards/:cardRef", ({ params }) => detailResponse(String(params.cardRef))),
  // Pages like the issuer: `cursor` is the last journalId seen; nextCursor is set while a full page came back.
  http.get("*/v1/cards/:cardRef/ledger", ({ params, request }) => {
    const query = new URL(request.url).searchParams;
    const all = mockLedger(String(params.cardRef));
    const limit = Number(query.get("limit") ?? 50);
    const cursor = query.get("cursor");
    const start = cursor === null ? 0 : all.findIndex((e) => e.journalId === cursor) + 1;
    const items = all.slice(start, start + limit);
    return HttpResponse.json({ items, nextCursor: items.length === limit ? (items.at(-1)?.journalId ?? null) : null });
  }),
  http.get("*/v1/cards/:cardRef/audit", ({ params, request }) => {
    const limit = Number(new URL(request.url).searchParams.get("limit") ?? 50);
    return HttpResponse.json({ items: (audit.get(String(params.cardRef)) ?? []).slice(0, limit), nextCursor: null });
  }),
  http.post("*/v1/cards/:cardRef/blocks", ({ params, request }) => {
    const cardRef = String(params.cardRef);
    withCard(cardRef, (c) => ({ ...c, status: "BLOCKED" }));
    recordAudit(cardRef, request, "CARD_BLOCKED");
    return detailResponse(cardRef);
  }),
  http.delete("*/v1/cards/:cardRef/blocks", ({ params, request }) => {
    const cardRef = String(params.cardRef);
    withCard(cardRef, (c) => ({ ...c, status: "ACTIVE" }));
    recordAudit(cardRef, request, "CARD_UNBLOCKED");
    return detailResponse(cardRef);
  }),
  http.put("*/v1/cards/:cardRef/limits", async ({ params, request }) => {
    const cardRef = String(params.cardRef);
    if (!state.has(cardRef)) return new HttpResponse(null, { status: 404 });
    if (request.headers.get("If-Match") !== etagOf(cardRef)) return staleEtag();
    const limits = (await request.json()) as CardLimits;
    withCard(cardRef, (c) => ({ ...c, limits, version: c.version + 1 }));
    recordAudit(cardRef, request, "CARD_LIMITS_UPDATED");
    return detailResponse(cardRef);
  }),
];
