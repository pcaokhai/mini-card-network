import { http, HttpResponse, type HttpHandler } from "msw";
import type { components } from "@/shared/api/generated/schema";

type ChaosScenario = components["schemas"]["ChaosScenario"];
type ChaosScenarioId = components["schemas"]["ChaosScenarioId"];
type ChaosRun = components["schemas"]["ChaosRun"];

// The canvas's renderVals(): 500.000.000 ₫ opening, 185.000 ₫ per transaction, per 100 transactions
// 6 declined and 8 / 12 / 5 reversed while "Mất câu trả lời" / "Mất kết nối" / "Câu trả lời đến muộn" is on.
const OPENING_MINOR = 500_000_000;
const AMOUNT_MINOR = 185_000;
const DECLINED_PER_100 = 6;
const REVERSED_PER_100: Partial<Record<ChaosScenarioId, number>> = { DROP_RESPONSE: 8, CONNECTION_CUT: 12, LATE_RESPONSE: 5 };
const RUNNING_MS = 3000;
const VERIFYING_MS = 800;

const IDS: ChaosScenarioId[] = ["SLOW_NETWORK", "CONNECTION_CUT", "DROP_RESPONSE", "DUPLICATE_REQUEST", "ISSUER_DOWN", "LATE_RESPONSE"];

interface MockRun {
  runId: string;
  requested: number;
  startedAt: number;
  reversedPer100: number;
}

// ponytail: module state, one browser tab's worth; the canvas opens with "Mất câu trả lời" on.
let enabled = new Set<ChaosScenarioId>(["DROP_RESPONSE"]);
const runs = new Map<string, MockRun>();

/** Tests start from the canvas's opening state. */
export function resetChaosMock() {
  enabled = new Set(["DROP_RESPONSE"]);
  runs.clear();
}

function scenario(id: ChaosScenarioId): ChaosScenario {
  return { id, enabled: enabled.has(id), easyText: id, technicalText: id };
}

/** Where a run is after `elapsedMs`: counts grow while RUNNING, money settles once it finishes. */
function snapshot(run: MockRun, elapsedMs: number): ChaosRun {
  const progress = Math.min(1, elapsedMs / RUNNING_MS);
  const completed = Math.round(run.requested * progress);
  const declined = Math.round((completed * DECLINED_PER_100) / 100);
  const reversed = Math.round((completed * run.reversedPer100) / 100);
  const approved = completed - declined - reversed;
  const status = progress < 1 ? "RUNNING" : elapsedMs < RUNNING_MS + VERIFYING_MS ? "VERIFYING" : "PASSED";
  return {
    runId: run.runId,
    status,
    requested: run.requested,
    completed,
    approved,
    declined,
    reversed,
    openingBalanceTotal: OPENING_MINOR,
    closingBalanceTotal: status === "PASSED" ? OPENING_MINOR - approved * AMOUNT_MINOR : 0,
    ledgerDiscrepancy: 0,
  };
}

/** Chaos Lab under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const chaosHandlers: HttpHandler[] = [
  http.get("*/v1/chaos/scenarios", () => HttpResponse.json(IDS.map(scenario))),
  http.put("*/v1/chaos/scenarios/:scenarioId", async ({ params, request }) => {
    const id = IDS.find((candidate) => candidate === params.scenarioId);
    if (!id) return new HttpResponse(null, { status: 404 });
    const body = (await request.json()) as { enabled: boolean };
    if (body.enabled) enabled.add(id);
    else enabled.delete(id);
    return HttpResponse.json(scenario(id));
  }),
  http.post("*/v1/chaos/runs", async ({ request }) => {
    const { transactions } = (await request.json()) as { transactions: number };
    const reversedPer100 = [...enabled].reduce((sum, id) => sum + (REVERSED_PER_100[id] ?? 0), 0);
    const run: MockRun = { runId: `run-${runs.size + 1}`, requested: transactions, startedAt: Date.now(), reversedPer100 };
    runs.set(run.runId, run);
    return HttpResponse.json(snapshot(run, 0), { status: 202 });
  }),
  http.get("*/v1/chaos/runs/:runId", ({ params }) => {
    const run = runs.get(String(params.runId));
    return run ? HttpResponse.json(snapshot(run, Date.now() - run.startedAt)) : new HttpResponse(null, { status: 404 });
  }),
];
