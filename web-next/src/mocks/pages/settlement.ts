import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";

type SettlementDay = components["schemas"]["SettlementDay"];
type SettlementStage = components["schemas"]["SettlementStage"];
type ReconBreak = components["schemas"]["ReconBreak"];
type TotalsRow = components["schemas"]["TotalsRow"];

const VND = "704";
const TOTAL_COUNT = 1282;
const NET_AMOUNT = 228_164_500;
/** Real stack: the gateway sends the 0500 after the cutover (MCN-702-AC2); dev:mock takes this long. */
export const TOTALS_EXCHANGE_DELAY_MS = 1200;

// The design canvas's totals (Settlement.dc.html `T`): acquirer (0500) vs issuer, amounts in VND.
const CANVAS_TOTALS: TotalsRow[] = [
  { metric: "DEBITS_COUNT", isoField: "76", acquirer: 1282, issuer: 1281, matches: false },
  { metric: "DEBITS_AMOUNT", isoField: "88", acquirer: 237_184_500, issuer: 237_149_500, matches: false },
  { metric: "DEBIT_REVERSALS_COUNT", isoField: "77", acquirer: 15, issuer: 14, matches: false },
  { metric: "DEBIT_REVERSALS_AMOUNT", isoField: "89", acquirer: 9_020_000, issuer: 8_420_000, matches: false },
  { metric: "NET_AMOUNT", isoField: "97", acquirer: NET_AMOUNT, issuer: 228_729_500, matches: false },
];

// The canvas's three breaks (`BR`), with its plain and technical descriptions.
const CANVAS_BREAKS: ReconBreak[] = [
  {
    breakId: "brk-626514000131",
    breakType: "MISSING_AT_ISSUER",
    rrn: "626514000131",
    amountDiff: { amount: 185_000, currency: VND },
    easyText: "Ngân hàng thanh toán có giao dịch này nhưng ngân hàng phát hành không có. Có thể message đã thất lạc trên đường đi.",
    technicalText: "acquirer.tran_log APPROVED · không có dòng tương ứng ở issuer.tran_log",
    resolution: "OPEN",
    resolvedBy: null,
  },
  {
    breakId: "brk-626514000124",
    breakType: "STATUS_MISMATCH",
    rrn: "626514000124",
    amountDiff: { amount: 600_000, currency: VND },
    easyText: "Một bên ghi đã hủy, bên kia vẫn ghi đã duyệt vì lệnh hủy còn nằm trong hàng đợi lúc khóa sổ.",
    technicalText: "acquirer REVERSED · issuer APPROVED · 0420 trong SAF tại thời điểm cutover",
    resolution: "OPEN",
    resolvedBy: null,
  },
  {
    breakId: "brk-626514000098",
    breakType: "AMOUNT_MISMATCH",
    rrn: "626514000098",
    amountDiff: { amount: 150_000, currency: VND },
    easyText: "Khách sạn chốt 1.350.000 ₫ nhưng ngân hàng phát hành vẫn ghi theo số tạm giữ 1.500.000 ₫.",
    technicalText: "0220 completion 1.350.000 vs auth_hold 1.500.000 chưa giải phóng phần dư",
    resolution: "OPEN",
    resolvedBy: null,
  },
];

const STAGES: SettlementStage[] = ["OPEN", "CUTOVER_DONE", "TOTALS_EXCHANGED", "RECONCILED", "FILE_GENERATED"];
const reached = (stage: SettlementStage, target: SettlementStage) => STAGES.indexOf(stage) >= STAGES.indexOf(target);

/** The day as dev:mock serves it at `stage`; also seeds the stories. */
export function settlementDayAt(businessDate: string, stage: SettlementStage, breaks: ReconBreak[] = CANVAS_BREAKS): SettlementDay {
  const reconciled = reached(stage, "RECONCILED");
  const openBreaks = reconciled ? breaks.filter((b) => b.resolution === "OPEN").length : 0;
  const allResolved = reconciled && openBreaks === 0;
  return {
    businessDate,
    stage,
    totals: reached(stage, "TOTALS_EXCHANGED")
      ? CANVAS_TOTALS.map((row) => (allResolved ? { ...row, issuer: row.acquirer, matches: true } : row))
      : [],
    matchedCount: reconciled ? TOTAL_COUNT - openBreaks : null,
    totalCount: reconciled ? TOTAL_COUNT : null,
    openBreaks,
    netPosition: { amount: NET_AMOUNT, currency: VND },
    clearingFile: reached(stage, "FILE_GENERATED")
      ? {
          fileName: `CLR_970499_${businessDate.replaceAll("-", "")}_001.csv`,
          recordCount: TOTAL_COUNT,
          total: { amount: NET_AMOUNT, currency: VND },
          sha256: `a91f03${"4b8d".repeat(13)}d27c2e`,
          status: "GENERATED",
        }
      : null,
  };
}

export const SETTLEMENT_BREAKS = CANVAS_BREAKS;

function problem(status: number, type: string, title: string, detail: string) {
  return HttpResponse.json(
    { type: `https://mcn.local/problems/${type}`, title, status, detail },
    { status, headers: { "Content-Type": "application/problem+json" } },
  );
}

/** One in-memory business day per handler set, so a test can start it at any stage. */
export function createSettlementHandlers(initialStage: SettlementStage = "TOTALS_EXCHANGED") {
  let stage = initialStage;
  let breaks = CANVAS_BREAKS.map((b) => ({ ...b }));
  let totalsDueAt = 0;
  const day = (businessDate: string) => {
    if (stage === "CUTOVER_DONE" && Date.now() >= totalsDueAt) stage = "TOTALS_EXCHANGED";
    return settlementDayAt(businessDate, stage, breaks);
  };
  const advance = (from: SettlementStage, to: SettlementStage, businessDate: string) => {
    if (stage !== from) return problem(409, "conflict", "Stage transition not allowed", `The day is ${stage}, not ${from}.`);
    stage = to;
    return HttpResponse.json(day(businessDate), { status: 202 });
  };

  return [
    http.get("*/v1/settlement/days/:businessDate", ({ params }) => HttpResponse.json(day(String(params.businessDate)))),
    http.post("*/v1/settlement/days/:businessDate/cutover", ({ params }) => {
      totalsDueAt = Date.now() + TOTALS_EXCHANGE_DELAY_MS;
      return advance("OPEN", "CUTOVER_DONE", String(params.businessDate));
    }),
    http.post("*/v1/settlement/days/:businessDate/reconciliations", ({ params }) =>
      advance("TOTALS_EXCHANGED", "RECONCILED", String(params.businessDate)),
    ),
    http.get("*/v1/settlement/days/:businessDate/breaks", () => HttpResponse.json(reached(stage, "RECONCILED") ? breaks : [])),
    http.post("*/v1/settlement/breaks/:breakId/resolutions", async ({ params, request }) => {
      const { resolution } = (await request.json()) as { resolution: ReconBreak["resolution"] };
      const found = breaks.find((b) => b.breakId === params.breakId);
      if (!found) return problem(404, "not-found", "Break not found", `No break ${String(params.breakId)}.`);
      const resolved: ReconBreak = { ...found, resolution, resolvedBy: "ops.demo" };
      breaks = breaks.map((b) => (b.breakId === found.breakId ? resolved : b));
      return HttpResponse.json(resolved);
    }),
    http.post("*/v1/settlement/days/:businessDate/clearing-files", ({ params }) => {
      if (stage !== "RECONCILED") return problem(409, "conflict", "Stage transition not allowed", `The day is ${stage}, not RECONCILED.`);
      const open = breaks.filter((b) => b.resolution === "OPEN").length;
      if (open > 0) {
        const detail = `Còn ${open} chênh lệch chưa xử lý. Hãy xử lý hết trước khi xuất file quyết toán.`;
        return problem(409, "open-breaks", "Open breaks remain", detail);
      }
      stage = "FILE_GENERATED";
      return HttpResponse.json(day(String(params.businessDate)).clearingFile, { status: 201 });
    }),
  ];
}

/** Settlement and reconciliation under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const settlementHandlers = createSettlementHandlers();
