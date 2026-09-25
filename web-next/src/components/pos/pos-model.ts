import type { EntryMode, Transaction } from "@/shared/api/pos-client";

export type TransactionType = "PURCHASE" | "PREAUTH" | "COMPLETION" | "REFUND" | "BALANCE";
export const TRANSACTION_TYPES: readonly TransactionType[] = ["PURCHASE", "PREAUTH", "COMPLETION", "REFUND", "BALANCE"];

export type CardTag = "normal" | "low" | "blocked" | "expired" | "limit" | "rich";

// Display-only copy of contracts/fixtures/cards.json: token, card reference and last four digits.
// The fixture's `pan` must never be embedded in frontend source (web-next/CLAUDE.md).
export const DISPLAY_CARDS = [
  { cardToken: "tok_normal", cardRef: "crd_normal0001", last4: "4417", tag: "normal" },
  { cardToken: "tok_low", cardRef: "crd_lowbal0002", last4: "9021", tag: "low" },
  { cardToken: "tok_blocked", cardRef: "crd_blockd0003", last4: "3310", tag: "blocked" },
  { cardToken: "tok_expired", cardRef: "crd_expird0004", last4: "7765", tag: "expired" },
  { cardToken: "tok_limit", cardRef: "crd_limit00005", last4: "1208", tag: "limit" },
  { cardToken: "tok_second", cardRef: "crd_second0006", last4: "5540", tag: "rich" },
] as const satisfies readonly { cardToken: string; cardRef: string; last4: string; tag: CardTag }[];
export type DisplayCard = (typeof DISPLAY_CARDS)[number];
export type CardToken = DisplayCard["cardToken"];

// DE 22 values the gateway sends for each EntryMode (gateway-go/internal/purchase/service.go).
// No PIN modes: the gateway never forwards a PIN block to the issuer yet (risk R-12).
export const ENTRY_MODES = [
  { id: "chip", entryMode: "CHIP_NO_PIN", de22: "052" },
  { id: "manual", entryMode: "MANUAL_NO_PIN", de22: "012" },
] as const satisfies readonly { id: string; entryMode: EntryMode; de22: string }[];
export type EntryId = (typeof ENTRY_MODES)[number]["id"];

// Outcomes the real issuer produces from the fixture cards (tok_limit has a 500 000 ₫ per-transaction limit).
export const SCENARIOS = [
  { id: "normal", cardToken: "tok_normal", amount: 250_000 },
  { id: "low", cardToken: "tok_low", amount: 350_000 },
  { id: "blocked", cardToken: "tok_blocked", amount: 90_000 },
  { id: "expired", cardToken: "tok_expired", amount: 150_000 },
  { id: "limit", cardToken: "tok_limit", amount: 600_000 },
] as const satisfies readonly { id: string; cardToken: CardToken; amount: number }[];
export type ScenarioId = (typeof SCENARIOS)[number]["id"];

export type Outcome =
  | "approved"
  | "rc51"
  | "rc54"
  | "rc61"
  | "rc62"
  | "rc91"
  | "declined"
  | "reversalPending"
  | "reversed";
export type ResultKind = "ok" | "bad" | "rev";

const KNOWN_DECLINES = new Set(["51", "54", "61", "62", "91"]);

export function outcomeOf(tx: Transaction): Outcome {
  if (tx.status === "APPROVED") return "approved";
  if (tx.status === "REVERSED") return "reversed";
  if (tx.status === "TIMED_OUT" || tx.status === "REVERSAL_PENDING") return "reversalPending";
  const rc = tx.responseCode ?? "";
  return KNOWN_DECLINES.has(rc) ? (`rc${rc}` as Outcome) : "declined";
}

export function kindOf(outcome: Outcome): ResultKind {
  if (outcome === "approved") return "ok";
  if (outcome === "reversed" || outcome === "reversalPending") return "rev";
  return "bad";
}

// Request/response MTI per type (gateway-go/internal/advtxn, docs/03 §6).
export const MTI: Record<Transaction["type"], [string, string]> = {
  PURCHASE: ["0200", "0210"],
  REFUND: ["0200", "0210"],
  BALANCE: ["0200", "0210"],
  PREAUTH: ["0100", "0110"],
  COMPLETION: ["0220", "0230"],
  REVERSAL: ["0420", "0430"],
};

export function techLine(tx: Transaction): string {
  const [request, response] = MTI[tx.type];
  const stan = tx.stan ?? "——————";
  const sent = `${request} STAN ${stan}`;
  const kind = kindOf(outcomeOf(tx));
  if (kind === "rev") {
    const end = tx.status === "REVERSED" ? "0430" : "SAF";
    return `${sent} → timeout 30s → 0420 (field 90 = ${request}${stan}…) → ${end}`;
  }
  const rc = tx.responseCode ? ` · RC ${tx.responseCode}` : "";
  const auth = tx.authCode ? ` · field 38 = ${tx.authCode}` : "";
  return `${sent} → ${response}${rc}${auth}`;
}

const MAX_AMOUNT_DIGITS = 10;

/** The canvas keypad: at most 10 digits, no leading zeros, C and a last ⌫ both leave "0". */
export function nextAmount(current: string, key: string): string {
  if (key === "C") return "0";
  if (key === "⌫") return current.length > 1 ? current.slice(0, -1) : "0";
  const next = (current === "0" ? "" : current) + key;
  if (next.length > MAX_AMOUNT_DIGITS) return current;
  return next.replace(/^0+/, "") || "0";
}
