import { describe, expect, it } from "vitest";
import { buildTopology, echoAge, endpointName, gatewayEventKey, linkTone, todaysEvents } from "./network-model";
import type { Link, NetworkEvent, SwitchStatus } from "@/shared/api/network-client";

const NOW = Date.parse("2026-09-21T14:41:10");

function link(linkId: string, from: string, to: string, status: Link["status"] = "SIGNED_ON"): Link {
  return { linkId, from, to, status, lastEchoAt: null, lastEchoOk: null, p99LatencyMs: null, inFlight: 0 };
}

const CANVAS_LINKS = [
  link("gateway-a", "gateway-a", "switch"),
  link("gateway-b", "gateway-b", "switch"),
  link("issuer-a", "switch", "issuer-a"),
  link("issuer-b", "switch", "issuer-b"),
];
const CLOSED: SwitchStatus = { circuit: "CLOSED", stipActive: false, stipLimit: { amount: 500_000, currency: "704" }, stipApprovedCount: 0 };

describe("linkTone", () => {
  it("MCN-205-AC1: colours each link status like the canvas badges", () => {
    expect(linkTone("SIGNED_ON")).toBe("ok");
    expect(linkTone("DOWN")).toBe("bad");
    expect(linkTone("CONNECTED")).toBe("warn");
    expect(linkTone("DISCONNECTED")).toBe("info");
  });
});

describe("echoAge", () => {
  const at = (secondsAgo: number) => new Date(NOW - secondsAgo * 1000).toISOString();

  it("MCN-205-AC2: buckets the last echo into just now, seconds, minutes and hours", () => {
    expect(echoAge({ ...link("a", "x", "y"), lastEchoAt: at(1), lastEchoOk: true }, NOW)).toEqual({ key: "justNow", n: 0 });
    expect(echoAge({ ...link("a", "x", "y"), lastEchoAt: at(12), lastEchoOk: true }, NOW)).toEqual({ key: "seconds", n: 12 });
    expect(echoAge({ ...link("a", "x", "y"), lastEchoAt: at(125), lastEchoOk: true }, NOW)).toEqual({ key: "minutes", n: 2 });
    expect(echoAge({ ...link("a", "x", "y"), lastEchoAt: at(7300), lastEchoOk: true }, NOW)).toEqual({ key: "hours", n: 2 });
  });

  it("MCN-205-AC2: reports no reply after a failed echo and nothing before the first one", () => {
    expect(echoAge({ ...link("a", "x", "y"), lastEchoAt: at(3), lastEchoOk: false }, NOW).key).toBe("noReply");
    expect(echoAge(link("a", "x", "y"), NOW).key).toBe("none");
  });
});

describe("endpointName", () => {
  it("turns endpoint ids into the canvas's instance names", () => {
    expect(endpointName("gateway-a")).toBe("Gateway A");
    expect(endpointName("issuer-b")).toBe("Issuer B");
    expect(endpointName("issuer")).toBe("Issuer");
  });
});

describe("buildTopology", () => {
  it("MCN-205-AC1: all four nodes and segments are up with the canvas's links", () => {
    const topo = buildTopology({ links: CANVAS_LINKS, switchStatus: CLOSED, terminalCount: 42 });
    expect(topo.nodes.map((n) => n.tone)).toEqual(["ok", "ok", "ok", "ok"]);
    expect(topo.segments).toEqual(["up", "up", "up"]);
    expect(topo.nodes[1]?.instances).toEqual(["gateway-a", "gateway-b"]);
    expect(topo.nodes[3]?.instances).toEqual(["issuer-a", "issuer-b"]);
  });

  it("MCN-804-AC2: issuer links down mark only the switch→issuer segment and a STIP switch", () => {
    const links = CANVAS_LINKS.map((l) => (l.to.startsWith("issuer") ? { ...l, status: "DOWN" as const } : l));
    const topo = buildTopology({ links, switchStatus: { ...CLOSED, circuit: "OPEN", stipActive: true }, terminalCount: 42 });
    expect(topo.segments).toEqual(["up", "up", "down"]);
    expect(topo.nodes.map((n) => n.tone)).toEqual(["ok", "ok", "warn", "bad"]);
  });

  it("a direct gateway→issuer link (real stack) drives both acquirer-side segments", () => {
    const topo = buildTopology({ links: [link("issuer", "gateway", "issuer", "DOWN")], switchStatus: undefined, terminalCount: undefined });
    expect(topo.segments).toEqual(["up", "down", "down"]);
    expect(topo.nodes[2]?.tone).toBe("unavailable");
    expect(topo.nodes[0]?.tone).toBe("unavailable");
    expect(topo.nodes[1]?.instances).toEqual(["gateway"]);
  });
});

describe("todaysEvents", () => {
  const event = (id: string, occurredAt: string): NetworkEvent => ({ id, occurredAt, severity: "INFO", easyText: id, technicalText: id });

  it("MCN-205-AC3: newest first, today only, capped at 30 rows", () => {
    const today = Array.from({ length: 40 }, (_, i) => event(`t${i}`, new Date(NOW - i * 60_000).toISOString()));
    const shuffled = [event("old", "2026-09-20T10:00:00"), ...today.slice().reverse()];
    const rows = todaysEvents(shuffled, NOW);
    expect(rows).toHaveLength(30);
    expect(rows[0]?.id).toBe("t0");
    expect(rows.some((r) => r.id === "old")).toBe(false);
  });
});

describe("gatewayEventKey", () => {
  it("MCN-205-AC3: maps every text gateway-go emits to a copy key and leaves unknown text alone", () => {
    expect(gatewayEventKey("Link to issuer is up")).toBe("linkUp");
    expect(gatewayEventKey("Link to issuer is down")).toBe("linkDown");
    expect(gatewayEventKey("Signed on again: the issuer had the link signed off")).toBe("signedOnAgain");
    expect(gatewayEventKey("Issuer still holds the link signed off")).toBe("stillSignedOff");
    expect(gatewayEventKey("A response arrived too late for a transaction")).toBe("lateResponse");
    expect(gatewayEventKey("Something new")).toBeUndefined();
  });
});
