import type { CardDetail, CardSummary, JournalEntry } from "@/shared/api/cards-client";
import { type CardTag, DISPLAY_CARDS } from "@/components/pos/pos-model";

export type StatusKind = "active" | "locked" | "expired";
type Hold = CardDetail["holds"][number];

// The design's slider ranges (Cards.dc.html): the daily ceiling in 1 000 000 ₫ steps, per transaction in 500 000 ₫.
export const LIMIT_RANGES = {
  daily: { min: 1_000_000, max: 50_000_000, step: 1_000_000 },
  perTransaction: { min: 500_000, max: 20_000_000, step: 500_000 },
} as const;

/** A card is valid through the last day of its MM/YY expiry month. */
function isPastExpiry(expiry: string, today: Date): boolean {
  const [month, year] = expiry.split("/").map(Number);
  if (month === undefined || year === undefined) return false;
  return today >= new Date(2000 + year, month, 1);
}

/**
 * The issuer only flags EXPIRED on a batch run, so a card past its expiry month still reads ACTIVE
 * there; the screen shows it expired, as the issuer's Validate participant would decline it (RC 54).
 */
export function statusKind(card: Pick<CardSummary, "status" | "expiry">, today: Date): StatusKind {
  if (card.status === "EXPIRED" || isPastExpiry(card.expiry, today)) return "expired";
  return card.status === "ACTIVE" ? "active" : "locked";
}

export function activeHoldTotal(holds: readonly Hold[]): number {
  return holds.filter((h) => h.status === "ACTIVE").reduce((sum, h) => sum + h.amount.amount, 0);
}

/** An unset ceiling comes back as 0 from the issuer, which reads as no usage bar at all. */
export function usagePercent(used: number, limit: number): number {
  return limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0;
}

export function cardTag(maskedPan: string): CardTag | undefined {
  return DISPLAY_CARDS.find((c) => maskedPan.endsWith(c.last4))?.tag;
}

export interface LedgerRow {
  journalId: string;
  occurredAt: string;
  entryType: JournalEntry["entryType"];
  /** null when the issuer only generated "<TYPE> journal <id>"; the screen names the type instead. */
  description: string | null;
  /** Signed change to the customer's account: negative is money out. */
  effect: number;
  debitAccount: string | null;
  creditAccount: string | null;
  amount: number;
}

const GENERATED_DESCRIPTION = /^[A-Z_]+ journal \S+$/;
// Customer accounts are ACC-<cardRef>; everything else is a GL account (docs/05 data model).
const isCustomer = (account: string) => account.startsWith("ACC-");

export function ledgerRow(entry: JournalEntry): LedgerRow {
  const debit = entry.postings.find((p) => p.direction === "DEBIT");
  const credit = entry.postings.find((p) => p.direction === "CREDIT");
  const effect = entry.postings
    .filter((p) => isCustomer(p.account))
    .reduce((sum, p) => sum + (p.direction === "DEBIT" ? -p.amount.amount : p.amount.amount), 0);
  return {
    journalId: entry.journalId,
    occurredAt: entry.occurredAt,
    entryType: entry.entryType,
    description: GENERATED_DESCRIPTION.test(entry.description) ? null : entry.description,
    effect,
    debitAccount: debit?.account ?? null,
    creditAccount: credit?.account ?? null,
    amount: credit?.amount.amount ?? 0,
  };
}
