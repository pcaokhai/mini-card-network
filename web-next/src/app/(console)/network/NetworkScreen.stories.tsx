import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useDisplayMode } from "@/shared/state/display-mode";
import type { Link, NetworkEvent, SafQueue, SwitchStatus } from "@/shared/api/network-client";
import { NetworkScreen } from "./NetworkScreen";

const secondsAgo = (s: number) => new Date(Date.now() - s * 1000).toISOString();
const todayAt = (h: number, m: number, s: number) => new Date(new Date().setHours(h, m, s, 0)).toISOString();

function link(linkId: string, from: string, to: string, p99: number, age: number, down = false): Link {
  return { linkId, from, to, status: down ? "DOWN" : "SIGNED_ON", lastEchoAt: secondsAgo(age), lastEchoOk: !down, p99LatencyMs: down ? null : p99, inFlight: 0 };
}

function links(issuerDown: boolean): Link[] {
  return [
    link("gateway-a", "gateway-a", "switch", 4, 12),
    link("gateway-b", "gateway-b", "switch", 5, 9),
    link("issuer-a", "switch", "issuer-a", 38, 7, issuerDown),
    link("issuer-b", "switch", "issuer-b", 41, 11, issuerDown),
  ];
}

const EVENTS: NetworkEvent[] = [
  { id: "3", occurredAt: todayAt(14, 30, 12), severity: "OK", easyText: "Xoay khóa mã hóa PIN thành công, hai bên xác nhận mã kiểm tra khớp.", technicalText: "0800/0810 70=161 · ZPK KCV 3F9A21 ACTIVE" },
  { id: "2", occurredAt: todayAt(13, 5, 40), severity: "INFO", easyText: "Gateway B khởi động lại sau khi cập nhật, tự đăng nhập lại mạng.", technicalText: "gateway-b restart · 0800 70=001 → 0810 RC 00" },
  { id: "1", occurredAt: todayAt(8, 0, 0), severity: "INFO", easyText: "Mở ngày giao dịch 21/09.", technicalText: "cutover 0800 70=201 · business date 0921" },
];

const vnd = (amount: number) => ({ amount, currency: "704" });
const SWITCH_OK: SwitchStatus = { circuit: "CLOSED", stipActive: false, stipLimit: vnd(500_000), stipApprovedCount: 0 };
const SAF_DOWN: SafQueue = {
  depth: 37,
  deadCount: 0,
  items: [
    { id: "1", mti: "0120", rrn: "626514000204", amount: vnd(320_000), attempts: 4, status: "PENDING", nextRetryAt: secondsAgo(-30) },
    { id: "2", mti: "0220", rrn: "626514000207", amount: vnd(95_000), attempts: 3, status: "PENDING", nextRetryAt: secondsAgo(-30) },
    { id: "3", mti: "0420", rrn: "626514000199", amount: vnd(450_000), attempts: 6, status: "PENDING", nextRetryAt: secondsAgo(-30) },
  ],
};
const TERMINALS = Array.from({ length: 42 }, (_, i) => ({ terminalId: `POS${i}`, merchantId: "M", merchantName: "M", mcc: "5814" }));

interface Seed {
  issuerDown?: boolean;
  backendGaps?: boolean;
}

function seeded({ issuerDown = false, backendGaps = false }: Seed) {
  return function WithSeed(Story: () => React.ReactElement) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity, refetchInterval: false } } });
    client.setQueryData(["network", "links"], backendGaps ? [link("issuer", "gateway", "issuer", 0, 40)] : links(issuerDown));
    client.setQueryData(["network", "events"], EVENTS);
    client.setQueryData(["network", "saf"], issuerDown ? SAF_DOWN : { depth: 0, deadCount: 0, items: [] });
    if (!backendGaps) {
      client.setQueryData(["network", "switch"], issuerDown ? { ...SWITCH_OK, circuit: "OPEN", stipActive: true, stipApprovedCount: 37 } : SWITCH_OK);
      client.setQueryData(["network", "terminals"], TERMINALS);
    }
    client.setQueryData(["chaos", "scenarios"], [{ id: "ISSUER_DOWN", enabled: issuerDown, easyText: "", technicalText: "" }]);
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };
}

const meta: Meta<typeof NetworkScreen> = { component: NetworkScreen, title: "Network/NetworkScreen" };
export default meta;
type Story = StoryObj<typeof NetworkScreen>;

const easy = () => {
  useDisplayMode.setState({ mode: "easy" });
};
const expert = () => {
  useDisplayMode.setState({ mode: "expert" });
};

export const Easy: Story = { decorators: [seeded({})], beforeEach: easy };
export const Expert: Story = { decorators: [seeded({})], beforeEach: expert };
export const IssuerDown: Story = { decorators: [seeded({ issuerDown: true })], beforeEach: easy };
export const IssuerDownExpert: Story = { decorators: [seeded({ issuerDown: true })], beforeEach: expert };
/** The real gateway today: one direct issuer link, no switch or terminal API. */
export const BackendGaps: Story = { decorators: [seeded({ backendGaps: true })], beforeEach: easy };
