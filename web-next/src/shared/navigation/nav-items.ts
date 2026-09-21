export type NavGroup = "main" | "lab" | "manage";

export interface NavItem {
  readonly href: string;
  readonly labelKey: string;
  readonly group: NavGroup;
  /** Release in which the screen's content ships (docs/07 §2). */
  readonly release: string;
}

/** MCN-004-AC1: nine entries in three groups, matching the design canvas. */
export const NAV_ITEMS = [
  { href: "/", labelKey: "overview", group: "main", release: "R3" },
  { href: "/pos", labelKey: "pos", group: "main", release: "R3" },
  { href: "/transactions", labelKey: "transactions", group: "main", release: "R3" },
  { href: "/lab/message", labelKey: "messageLab", group: "lab", release: "R1" },
  { href: "/lab/chaos", labelKey: "chaosLab", group: "lab", release: "R4" },
  { href: "/cards", labelKey: "cards", group: "manage", release: "R3" },
  { href: "/security", labelKey: "security", group: "manage", release: "R5" },
  { href: "/settlement", labelKey: "settlement", group: "manage", release: "R7" },
  { href: "/network", labelKey: "network", group: "manage", release: "R2" },
] as const satisfies readonly NavItem[];

export const NAV_GROUPS: readonly NavGroup[] = ["main", "lab", "manage"];

export function releaseFor(href: string): string {
  const item = NAV_ITEMS.find((i) => i.href === href);
  if (item === undefined) throw new Error(`Unknown nav href ${href}`);
  return item.release;
}
