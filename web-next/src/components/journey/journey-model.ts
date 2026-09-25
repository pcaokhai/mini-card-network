import type { Journey } from "@/shared/api/journey-client";
import type { JournalEntry } from "@/shared/api/cards-client";

type Step = Journey["steps"][number];
type Status = Journey["transaction"]["status"];

export type Outcome = "approved" | "declined" | "autoReversed" | "cancelled" | "reversing" | "pending";

/** Which story the journey tells; drives the summary headline, icon and money note. */
export function outcomeOf(status: Status, steps: readonly Step[]): Outcome {
  switch (status) {
    case "APPROVED":
      return "approved";
    case "DECLINED":
      return "declined";
    case "REVERSED":
      return steps.some((s) => s.code === "NO_RESPONSE") ? "autoReversed" : "cancelled";
    case "REVERSAL_PENDING":
    case "TIMED_OUT":
      return "reversing";
    default:
      return "pending";
  }
}

export type Tone = "ok" | "bad" | "rev" | "warn";

export const OUTCOME_TONE: Record<Outcome, Tone> = {
  approved: "ok",
  declined: "bad",
  autoReversed: "rev",
  cancelled: "rev",
  reversing: "warn",
  pending: "warn",
};

export const KIND_TONE: Record<Step["kind"], Tone | "info"> = {
  OK: "ok",
  BAD: "bad",
  WARN: "warn",
  REVERSAL: "rev",
  INFO: "info",
};

const MS_PER_SECOND = 1000;

/** Offset from the first event, as the canvas writes it: "+182 ms", "+30,09 s". */
export function formatOffset(ms: number, locale: string): string {
  if (ms < MS_PER_SECOND) return `+${ms} ms`;
  const seconds = (ms / MS_PER_SECOND).toLocaleString(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return `+${seconds} s`;
}

/** The customer account's signed effect of one journal: a debit takes money out. */
function customerEffect(entry: JournalEntry): number {
  return entry.postings
    .filter((p) => p.account.startsWith("ACC-"))
    .reduce((sum, p) => sum + (p.direction === "DEBIT" ? -p.amount.amount : p.amount.amount), 0);
}

export interface Balances {
  before: number;
  final: number;
  /** This transaction's own journals, oldest first, as signed effects on the customer's account. */
  own: number[];
}

/**
 * Rewinds the account's current ledger balance through its journals (newest first) to just before
 * this transaction's first journal. Null when the issuer never journaled it (a decline).
 */
export function customerBalances(newestFirst: readonly JournalEntry[], currentBalance: number, rrn: string): Balances | null {
  const oldestOwn = newestFirst.map((e) => e.rrn).lastIndexOf(rrn);
  if (oldestOwn < 0) return null;
  const sinceStart = newestFirst.slice(0, oldestOwn + 1);
  const before = currentBalance - sinceStart.reduce((sum, e) => sum + customerEffect(e), 0);
  const own = sinceStart.filter((e) => e.rrn === rrn).map(customerEffect).reverse();
  return { before, final: before + own.reduce((a, b) => a + b, 0), own };
}

export interface MoneyRow {
  kind: "before" | "debit" | "credit" | "final";
  amount: number;
  atIndex: number;
}

type MoneyJourney = Pick<Journey, "steps" | "money"> & { transaction: Pick<Journey["transaction"], "status"> };

/**
 * The money panel's rows, each revealed once playback reaches its step index. With the issuer's
 * ledger the debit and credit are its real journals (a timed-out purchase was still debited, which
 * the acquirer never saw); without it, the provider's deltas. A decline moves no money.
 */
export function moneyRows(journey: MoneyJourney, balances: Balances | null): MoneyRow[] {
  if (journey.transaction.status === "DECLINED") return [];
  if (!balances) return providerDeltas(journey);
  const index = (...codes: NonNullable<Step["code"]>[]) => {
    const found = journey.steps.findIndex((s) => s.code !== undefined && codes.includes(s.code));
    return found < 0 ? journey.steps.length - 1 : found;
  };
  const moves: MoneyRow[] = balances.own
    .filter((amount) => amount !== 0)
    .map((amount) =>
      amount < 0
        ? { kind: "debit", amount, atIndex: index("ISSUER_APPROVED", "REQUEST_SENT") }
        : { kind: "credit", amount, atIndex: index("REVERSAL_CONFIRMED") },
    );
  const lastIndex = moves.length ? Math.max(...moves.map((m) => m.atIndex)) : journey.steps.length - 1;
  return [
    { kind: "before", amount: balances.before, atIndex: 0 },
    ...moves,
    { kind: "final", amount: balances.final, atIndex: lastIndex },
  ];
}

function providerDeltas(journey: MoneyJourney): MoneyRow[] {
  const indexOfSeq = (seq: number) => Math.max(0, journey.steps.findIndex((s) => s.seq === seq));
  return journey.money
    .filter((m) => m.delta !== 0)
    .map((m) => ({ kind: m.delta < 0 ? "debit" : "credit", amount: m.delta, atIndex: indexOfSeq(m.atStep) }));
}
