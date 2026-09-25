"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { SVGProps } from "react";
import { NAV_GROUPS, NAV_ITEMS } from "@/shared/navigation/nav-items";
import { BusinessDateCard } from "@/shared/ui/BusinessDateCard";
import {
  BoltIcon,
  CardIcon,
  HomeIcon,
  JourneyIcon,
  LedgerIcon,
  LockIcon,
  MessageIcon,
  NetworkIcon,
  TerminalIcon,
} from "@/shared/ui/icons";

const NAV_ICONS: Record<string, (props: SVGProps<SVGSVGElement>) => React.ReactElement> = {
  "/": HomeIcon,
  "/pos": TerminalIcon,
  "/transactions": JourneyIcon,
  "/lab/message": MessageIcon,
  "/lab/chaos": BoltIcon,
  "/cards": CardIcon,
  "/security": LockIcon,
  "/settlement": LedgerIcon,
  "/network": NetworkIcon,
};

export function Sidebar() {
  const pathname = usePathname();
  const t = useTranslations();
  return (
    <nav
      aria-label="Main"
      className="flex w-62 shrink-0 flex-col gap-0.5 border-r border-border bg-surface px-3.5 py-5"
    >
      <div className="flex items-center gap-2.5 px-2.5 pt-1 pb-[18px]">
        <span className="flex size-[34px] shrink-0 items-center justify-center rounded-[10px] bg-accent text-white">
          <CardIcon />
        </span>
        <span>
          <span className="block text-[15px] font-semibold">{t("app.name")}</span>
          <span className="block text-xs text-muted">{t("app.tagline")}</span>
        </span>
      </div>
      {NAV_GROUPS.map((group) => (
        <section key={group} aria-labelledby={`nav-${group}`}>
          <h2
            id={`nav-${group}`}
            // The design canvas gives the first group no visible heading; the others label the split.
            className={
              group === "main"
                ? "sr-only"
                : "px-3 pt-4 pb-1.5 text-xs font-semibold text-[#6B6D75]"
            }
          >
            {t(`nav.groups.${group}`)}
          </h2>
          <ul>
            {NAV_ITEMS.filter((item) => item.group === group).map((item) => {
              const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
              const ItemIcon = NAV_ICONS[item.href] ?? HomeIcon;
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={
                      active
                        ? "flex h-10 items-center gap-3 rounded-[10px] bg-accent-soft px-3 text-sm font-semibold text-accent"
                        : "flex h-10 items-center gap-3 rounded-[10px] px-3 text-sm text-[#3A3C43] hover:bg-canvas"
                    }
                  >
                    <ItemIcon className="shrink-0" />
                    {t(`nav.${item.labelKey}`)}
                  </Link>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
      <BusinessDateCard />
    </nav>
  );
}
