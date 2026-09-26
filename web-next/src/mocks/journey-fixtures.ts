import type { components } from "@/shared/api/generated/schema";

type Journey = components["schemas"]["Journey"];
type Step = Journey["steps"][number];
type IsoField = components["schemas"]["IsoField"];
type JournalEntry = components["schemas"]["JournalEntry"];
type CardSummary = components["schemas"]["CardSummary"];

// Journeys shaped like the gateway's (MCN-304, StepCode) and valued like the design canvas's
// scenarios (project/Journey.dc.html), for `pnpm dev:mock`, tests and stories. Card numbers are
// masked; the frontend never holds a PAN (web-next/CLAUDE.md).

const F = (de: string, easyName: string, technicalName: string, value: string): IsoField => ({
  de, easyName, technicalName, format: "", value,
});

interface MessageData {
  pan: string;
  amount: string;
  stan: string;
  de7: string;
  rrn: string;
}

function message(mti: string, d: MessageData, rc?: string): NonNullable<Step["message"]> {
  const fields = [
    F("MTI", "Message type", "Message type indicator", mti),
    F("2", "Card number", "PAN", d.pan),
    F("3", "Transaction type", "Processing code", "000000"),
    F("4", "Amount", "Amount, transaction", d.amount),
    F("7", "Sent at", "Transmission date/time", d.de7),
    F("11", "Trace number", "STAN", d.stan),
  ];
  if (mti === "0200") fields.push(F("22", "Card read", "POS entry mode", "051"));
  fields.push(F("37", "Reference", "RRN", d.rrn));
  if (mti === "0210" && rc === "00") fields.push(F("38", "Approval code", "Authorization ID", `A${d.stan.slice(1)}`));
  if (rc) fields.push(F("39", "Result code", "Response code", rc));
  fields.push(F("41", "POS ID", "Terminal ID", "00000042"), F("49", "Currency", "Currency code", "704"));
  if (mti.startsWith("042")) fields.push(F("90", "Original transaction", "Original data elements", `0200${d.stan}${d.de7}0000097049900000000000`));
  return { mti, fields };
}

const step = (seq: number, code: Step["code"], actor: Step["actor"], offsetMs: number, kind: Step["kind"], technicalText: string, msg: Step["message"] = null): Step => ({
  seq, code, actor, offsetMs, kind, technicalText, title: code ?? "", easyText: "", message: msg,
});

const txn = (rrn: string, status: Journey["transaction"]["status"], amount: number, last4: string, merchantName: string, responseCode: string | null): Journey["transaction"] => ({
  rrn, status, type: "PURCHASE", responseCode, responseLabel: null, amount: { amount, currency: "704" },
  maskedPan: `970436******${last4}`, terminalId: "00000042", merchantName, createdAt: "2026-09-21T07:32:08Z",
});

const approvedData = { pan: "970436******4417", amount: "000000250000", stan: "000123", de7: "0921073208", rrn: "626514000123" };
export const APPROVED_JOURNEY: Journey = {
  transaction: txn("626514000123", "APPROVED", 250_000, "4417", "Cà phê Góc Phố", "00"),
  steps: [
    step(1, "POS_REQUEST", "POS", 0, "OK", "POST /v1/transactions/purchases · entry mode 051"),
    step(2, "REQUEST_SENT", "ACQUIRER", 6, "OK", "Build 0200 · STAN 000123 · MAC · MUX key = STAN + DE 7", message("0200", approvedData)),
    step(3, "ISSUER_APPROVED", "ISSUER", 168, "OK", "0210 · RC 00 · field 38 = A00123", message("0210", approvedData, "00")),
    step(4, "POS_RESULT", "POS", 182, "OK", "MUX match → state APPROVED → WebSocket push"),
  ],
  money: [{ label: "Purchase", delta: -250_000, balanceAfter: null, atStep: 3 }],
};

const reversedData = { pan: "970436******5540", amount: "000000600000", stan: "000124", de7: "0921073244", rrn: "626514000124" };
export const AUTO_REVERSED_JOURNEY: Journey = {
  transaction: { ...txn("626514000124", "REVERSED", 600_000, "5540", "Trạm xăng Bến Nghé", null), reversalReason: "TIMEOUT" },
  steps: [
    step(1, "POS_REQUEST", "POS", 0, "OK", "POST /v1/transactions/purchases · entry mode 051"),
    step(2, "REQUEST_SENT", "ACQUIRER", 8, "OK", "Build 0200 · STAN 000124 · start timer 30 s", message("0200", reversedData)),
    step(3, "NO_RESPONSE", "ACQUIRER", 30_000, "WARN", "context deadline exceeded · state SENT → TIMED_OUT"),
    step(4, "REVERSAL_QUEUED", "SAF", 30_010, "REVERSAL", "INSERT saf_queue (0420, PENDING) · state → REVERSAL_PENDING"),
    step(5, "POS_RESULT", "POS", 30_012, "BAD", "HTTP 200 · status TIMED_OUT · POS shows a connection error"),
    step(6, "REVERSAL_SENT", "ACQUIRER", 30_020, "REVERSAL", "0420 · field 90 → 0200 STAN 000124 · attempts 1", message("0420", { ...reversedData, stan: "000125", de7: "0921073314" }, "68")),
    step(7, "REVERSAL_CONFIRMED", "ISSUER", 30_090, "OK", "0430 RC 00 · SAF ACKED · state → REVERSED", message("0430", { ...reversedData, stan: "000125", de7: "0921073314" }, "00")),
  ],
  money: [{ label: "Refund", delta: 600_000, balanceAfter: null, atStep: 7 }],
};

const declinedData = { pan: "970436******9021", amount: "000000350000", stan: "000126", de7: "0921073402", rrn: "626514000126" };
export const DECLINED_JOURNEY: Journey = {
  transaction: txn("626514000126", "DECLINED", 350_000, "9021", "Nhà sách Ánh Dương", "51"),
  steps: [
    step(1, "POS_REQUEST", "POS", 0, "OK", "POST /v1/transactions/purchases · entry mode 051"),
    step(2, "REQUEST_SENT", "ACQUIRER", 7, "OK", "Build 0200 · STAN 000126", message("0200", declinedData)),
    step(3, "ISSUER_DECLINED", "ISSUER", 141, "BAD", "0210 · RC 51 · available balance below the amount", message("0210", declinedData, "51")),
    step(4, "POS_RESULT", "POS", 150, "BAD", "MUX match → state DECLINED → WebSocket push"),
  ],
  money: [],
};

export const JOURNEYS: Record<string, Journey> = Object.fromEntries(
  [APPROVED_JOURNEY, AUTO_REVERSED_JOURNEY, DECLINED_JOURNEY].map((j) => [j.transaction.rrn, j]),
);

// The issuer side of the same story: the cards and each one's journals, newest first.
export const MOCK_CARDS: (CardSummary & { balance: number })[] = [
  { cardRef: "crd_normal0001", maskedPan: "970436******4417", holderName: "Nguyen Minh Anh", status: "ACTIVE", expiry: "11/28", balance: 4_750_000 },
  { cardRef: "crd_lowbal0002", maskedPan: "970436******9021", holderName: "Tran Thu Ha", status: "ACTIVE", expiry: "03/29", balance: 80_000 },
  { cardRef: "crd_blockd0003", maskedPan: "970436******3310", holderName: "Le Quoc Bao", status: "BLOCKED", expiry: "07/27", balance: 2_000_000 },
  { cardRef: "crd_expird0004", maskedPan: "970436******7765", holderName: "Pham Gia Huy", status: "ACTIVE", expiry: "08/26", balance: 1_500_000 },
  { cardRef: "crd_limit00005", maskedPan: "970436******1208", holderName: "Vo Thanh Tam", status: "ACTIVE", expiry: "12/29", balance: 50_000_000 },
  { cardRef: "crd_second0006", maskedPan: "970436******5540", holderName: "Dang Ngoc Linh", status: "ACTIVE", expiry: "06/30", balance: 200_000_000 },
];

function journal(journalId: string, rrn: string, cardRef: string, entryType: JournalEntry["entryType"], customer: "DEBIT" | "CREDIT", amount: number): JournalEntry {
  const other = customer === "DEBIT" ? "CREDIT" : "DEBIT";
  return {
    journalId, rrn, occurredAt: "2026-09-21T07:32:08Z", description: `${entryType} journal ${journalId}`, entryType,
    postings: [
      { account: `ACC-${cardRef}`, direction: customer, amount: { amount, currency: "704" } },
      { account: "SETTLEMENT_SUSPENSE", direction: other, amount: { amount, currency: "704" } },
    ],
  };
}

export const MOCK_LEDGERS: Record<string, JournalEntry[]> = {
  crd_normal0001: [journal("3", "626514000123", "crd_normal0001", "PURCHASE", "DEBIT", 250_000)],
  crd_second0006: [
    journal("5", "626514000124", "crd_second0006", "REVERSAL", "CREDIT", 600_000),
    journal("4", "626514000124", "crd_second0006", "PURCHASE", "DEBIT", 600_000),
  ],
};
