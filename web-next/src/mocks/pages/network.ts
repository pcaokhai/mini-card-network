import { http, HttpResponse } from "msw";
import type { components } from "@/shared/api/generated/schema";

type Link = components["schemas"]["Link"];
type NetworkEvent = components["schemas"]["NetworkEvent"];
type SafItem = components["schemas"]["SafItem"];
type SwitchStatus = components["schemas"]["SwitchStatus"];
type ChaosScenario = components["schemas"]["ChaosScenario"];

const vnd = (amount: number) => ({ amount, currency: "704" });

// The canvas's four ISO links: [linkId, from, to, p99 ms, seconds since the last echo].
const LINKS = [
  ["gateway-a", "gateway-a", "switch", 4, 12],
  ["gateway-b", "gateway-b", "switch", 5, 9],
  ["issuer-a", "switch", "issuer-a", 38, 7],
  ["issuer-b", "switch", "issuer-b", 41, 11],
] as const;
// ponytail: echoes repeat on a fixed cycle so the "last echo" ages stay in the canvas's range.
const ECHO_CYCLE_MS = 15_000;

const state = {
  issuerDown: false,
  recovered: false,
  echoBase: new Map<string, number>(LINKS.map(([id, , , , age]) => [id, Date.now() - age * 1000])),
};

const isIssuerLink = (to: string) => to.startsWith("issuer");

function links(): Link[] {
  const now = Date.now();
  return LINKS.map(([linkId, from, to, p99]) => {
    const down = state.issuerDown && isIssuerLink(to);
    const base = state.echoBase.get(linkId) ?? now;
    return {
      linkId,
      from,
      to,
      status: down ? "DOWN" : "SIGNED_ON",
      lastEchoAt: new Date(now - ((now - base) % ECHO_CYCLE_MS)).toISOString(),
      lastEchoOk: !down,
      p99LatencyMs: down ? null : p99,
      inFlight: 0,
    };
  });
}

/** Canvas times are wall-clock times of the simulated business day. */
function todayAt(time: string): string {
  const [h = 0, m = 0, s = 0] = time.split(":").map(Number);
  const at = new Date();
  at.setHours(h, m, s, 0);
  return at.toISOString();
}

function event(id: string, time: string, severity: NetworkEvent["severity"], easyText: string, technicalText: string): NetworkEvent {
  return { id, occurredAt: todayAt(time), severity, easyText, technicalText };
}

const BASE_EVENTS = [
  event("e3", "14:30:12", "OK", "Xoay khóa mã hóa PIN thành công, hai bên xác nhận mã kiểm tra khớp.", "0800/0810 70=161 · ZPK KCV 3F9A21 ACTIVE"),
  event("e2", "13:05:40", "INFO", "Gateway B khởi động lại sau khi cập nhật, tự đăng nhập lại mạng.", "gateway-b restart · 0800 70=001 → 0810 RC 00"),
  event("e1", "08:00:00", "INFO", "Mở ngày giao dịch 21/09.", "cutover 0800 70=201 · business date 0921"),
];
const DOWN_EVENTS = [
  event("e5", "14:41:05", "WARN", "Ngắt mạch, chuyển sang duyệt thay cho các giao dịch nhỏ.", "circuit CLOSED → OPEN · STIP ACTIVE"),
  event("e4", "14:41:02", "ERROR", "Issuer A và B không trả lời kiểm tra kết nối 3 lần liên tiếp.", "echo 0800 70=301 timeout ×3 · link DOWN"),
];
const RECOVERED_EVENT = event(
  "e6",
  "14:44:30",
  "OK",
  "Ngân hàng phát hành hoạt động trở lại. 37 thông báo tồn đọng đã được gửi và xác nhận.",
  "circuit HALF_OPEN → CLOSED · SAF drained 37 advice (0120/0220)",
);

function events(): NetworkEvent[] {
  if (state.issuerDown) return [...DOWN_EVENTS, ...BASE_EVENTS];
  return state.recovered ? [RECOVERED_EVENT, ...BASE_EVENTS] : BASE_EVENTS;
}

const STIP_APPROVED = 37;

function safItem(id: string, mti: SafItem["mti"], rrn: string, amount: number, attempts: number): SafItem {
  return { id, mti, rrn, amount: vnd(amount), attempts, status: "PENDING", nextRetryAt: new Date(Date.now() + 30_000).toISOString(), lastError: "issuer timeout" };
}

const SAF_ITEMS = [
  safItem("saf-1", "0120", "626514000204", 320_000, 4),
  safItem("saf-2", "0220", "626514000207", 95_000, 3),
  safItem("saf-3", "0420", "626514000199", 450_000, 6),
];

function switchStatus(): SwitchStatus {
  return {
    circuit: state.issuerDown ? "OPEN" : "CLOSED",
    stipActive: state.issuerDown,
    stipLimit: vnd(500_000),
    stipApprovedCount: state.issuerDown ? STIP_APPROVED : 0,
  };
}

const TERMINAL_COUNT = 42;
const TERMINALS = Array.from({ length: TERMINAL_COUNT }, (_, i) => ({
  terminalId: `POS${String(i + 1).padStart(5, "0")}`,
  merchantId: "MCN000000000001",
  merchantName: "Cà phê Góc Phố",
  mcc: "5814",
}));

// ponytail: the Chaos Lab has no mock of its own yet; move these into chaos.ts when it gets one.
const CHAOS_IDS: ChaosScenario["id"][] = ["SLOW_NETWORK", "CONNECTION_CUT", "DROP_RESPONSE", "DUPLICATE_REQUEST", "ISSUER_DOWN", "LATE_RESPONSE"];
const chaosEnabled = new Set<ChaosScenario["id"]>();
const scenario = (id: ChaosScenario["id"]): ChaosScenario => ({ id, enabled: chaosEnabled.has(id), easyText: "", technicalText: id });

/** Network operations under `pnpm dev:mock`. Values mirror the design canvas; this page's agent owns this file. */
export const networkHandlers = [
  http.get("*/v1/network/links", () => HttpResponse.json(links())),
  http.post("*/v1/network/links/:linkId/echo", ({ params }) => {
    const linkId = String(params.linkId);
    const found = links().find((l) => l.linkId === linkId);
    if (!found) return new HttpResponse(null, { status: 404 });
    const ok = found.status === "SIGNED_ON";
    if (ok) state.echoBase.set(linkId, Date.now());
    return HttpResponse.json({ ok, latencyMs: ok ? found.p99LatencyMs : null, responseCode: ok ? "00" : null });
  }),
  http.get("*/v1/network/saf", () =>
    HttpResponse.json(state.issuerDown ? { depth: STIP_APPROVED, deadCount: 0, items: SAF_ITEMS } : { depth: 0, deadCount: 0, items: [] }),
  ),
  http.get("*/v1/network/switch", () => HttpResponse.json(switchStatus())),
  http.get("*/v1/network/events", () => HttpResponse.json({ items: events(), nextCursor: null })),
  http.get("*/v1/terminals", () => HttpResponse.json(TERMINALS)),
  http.get("*/v1/chaos/scenarios", () => HttpResponse.json(CHAOS_IDS.map(scenario))),
  http.put("*/v1/chaos/scenarios/:scenarioId", async ({ params, request }) => {
    const id = String(params.scenarioId) as ChaosScenario["id"];
    if (!CHAOS_IDS.includes(id)) return new HttpResponse(null, { status: 404 });
    const { enabled } = (await request.json()) as { enabled: boolean };
    if (enabled) chaosEnabled.add(id);
    else chaosEnabled.delete(id);
    // Transition-based so a replayed request (the browser worker can resolve one twice) changes nothing.
    if (id === "ISSUER_DOWN" && enabled !== state.issuerDown) {
      state.recovered = !enabled;
      state.issuerDown = enabled;
    }
    return HttpResponse.json(scenario(id));
  }),
];

/** Tests share this module's state; put the canvas's opening scene back between them. */
export function resetNetworkMock() {
  state.issuerDown = false;
  state.recovered = false;
  chaosEnabled.clear();
}
