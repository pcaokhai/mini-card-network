import type { Link, NetworkEvent, SwitchStatus } from "@/shared/api/network-client";

/** The canvas's tone table: ok / bad / warn / info, plus a neutral state when the backend has no data. */
export type Tone = "ok" | "bad" | "warn" | "info" | "unavailable";

export type Segment = "up" | "down";

export interface TopologyNode {
  id: "pos" | "acquirer" | "switch" | "issuer";
  tone: Tone;
  /** Endpoint ids behind the node, e.g. `gateway-a`, `issuer-b`. */
  instances: string[];
  /** Registered terminals for the POS node, running instances otherwise. */
  count: number;
}

export interface EchoAge {
  key: "noReply" | "none" | "justNow" | "seconds" | "minutes" | "hours";
  n: number;
}

const LINK_TONE: Record<Link["status"], Tone> = { SIGNED_ON: "ok", CONNECTED: "warn", DOWN: "bad", DISCONNECTED: "info" };
const JUST_NOW_SECONDS = 3;
const MINUTE = 60;
const HOUR = 3600;
const MAX_EVENTS = 30;

export function linkTone(status: Link["status"]): Tone {
  return LINK_TONE[status];
}

export function echoAge(link: Link, now: number): EchoAge {
  if (link.lastEchoOk === false) return { key: "noReply", n: 0 };
  const at = link.lastEchoAt ? Date.parse(link.lastEchoAt) : NaN;
  if (Number.isNaN(at)) return { key: "none", n: 0 };
  const seconds = Math.max(0, Math.floor((now - at) / 1000));
  if (seconds < JUST_NOW_SECONDS) return { key: "justNow", n: 0 };
  if (seconds < MINUTE) return { key: "seconds", n: seconds };
  if (seconds < HOUR) return { key: "minutes", n: Math.floor(seconds / MINUTE) };
  return { key: "hours", n: Math.floor(seconds / HOUR) };
}

/** `gateway-a` → `Gateway A`: the canvas names instances by their id with a capital letter per word. */
export function endpointName(id: string): string {
  return id
    .split("-")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

const isUp = (link: Link) => link.status === "SIGNED_ON";
const unique = (ids: string[]) => [...new Set(ids)];

export function buildTopology(input: {
  links: Link[];
  switchStatus: SwitchStatus | undefined;
  terminalCount: number | undefined;
}): { nodes: TopologyNode[]; segments: Segment[] } {
  const { links, switchStatus, terminalCount } = input;
  const issuerLinks = links.filter((l) => l.to.startsWith("issuer"));
  const switchLinks = links.filter((l) => l.to.startsWith("switch"));
  // The real gateway talks to the issuer directly, so its issuer link also stands for acquirer→switch.
  const acquirerLinks = switchLinks.length > 0 ? switchLinks : issuerLinks;
  const issuerUp = issuerLinks.some(isUp);
  const acquirerUp = acquirerLinks.some(isUp);

  let switchTone: Tone = "unavailable";
  if (switchStatus) switchTone = switchStatus.circuit !== "CLOSED" || switchStatus.stipActive ? "warn" : "ok";

  const acquirers = unique(acquirerLinks.map((l) => l.from));
  const issuers = unique(issuerLinks.map((l) => l.to));
  return {
    nodes: [
      { id: "pos", tone: terminalCount === undefined ? "unavailable" : "ok", instances: [], count: terminalCount ?? 0 },
      { id: "acquirer", tone: acquirerUp ? "ok" : "bad", instances: acquirers, count: acquirers.length },
      { id: "switch", tone: switchTone, instances: [], count: 0 },
      { id: "issuer", tone: issuerUp ? "ok" : "bad", instances: issuers, count: issuerLinks.filter(isUp).length },
    ],
    // POS → acquirer is the browser's own path through the BFF: if links answered, it is up.
    segments: ["up", acquirerUp ? "up" : "down", issuerUp ? "up" : "down"],
  };
}

function isSameLocalDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

export function todaysEvents(events: NetworkEvent[], now: number): NetworkEvent[] {
  const today = new Date(now);
  return events
    .filter((e) => isSameLocalDay(new Date(e.occurredAt), today))
    .sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))
    .slice(0, MAX_EVENTS);
}

// Codes with Vietnamese copy in network.events.known (contracts NetworkEventCode). The gateway's
// easyText is its English fallback; a row without a code (written before the code existed) shows it.
const KNOWN_EVENT_CODES = new Set<string>([
  "LINK_UP",
  "LINK_DOWN",
  "SIGNED_ON",
  "SIGNED_OFF",
  "SIGNED_ON_AGAIN",
  "SIGN_ON_FAILED",
  "ECHO_OK",
  "ECHO_FAILED",
  "LATE_RESPONSE",
]);

/** The copy key for an event, from its language-neutral code (NET-G15); undefined keeps the provider text. */
export function networkEventKey(event: NetworkEvent): string | undefined {
  return event.code && KNOWN_EVENT_CODES.has(event.code) ? event.code : undefined;
}
