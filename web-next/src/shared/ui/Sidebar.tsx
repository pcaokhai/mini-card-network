"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { NAV_GROUPS, NAV_ITEMS } from "@/shared/navigation/nav-items";

export function Sidebar() {
  const pathname = usePathname();
  const t = useTranslations();
  return (
    <nav aria-label="Main" className="flex w-62 shrink-0 flex-col gap-0.5 border-r border-border bg-surface px-3.5 py-5">
      <div className="px-2.5 pb-4">
        <div className="text-[15px] font-semibold">{t("app.name")}</div>
        <div className="text-xs text-muted">{t("app.tagline")}</div>
      </div>
      {NAV_GROUPS.map((group) => (
        <section key={group} aria-labelledby={`nav-${group}`}>
          <h2 id={`nav-${group}`} className="px-3 pt-4 pb-1.5 text-xs font-semibold text-muted">
            {t(`nav.groups.${group}`)}
          </h2>
          <ul>
            {NAV_ITEMS.filter((item) => item.group === group).map((item) => {
              const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={
                      active
                        ? "flex h-10 items-center rounded-[10px] bg-accent-soft px-3 text-sm font-semibold text-accent"
                        : "flex h-10 items-center rounded-[10px] px-3 text-sm text-[#3A3C43] hover:bg-canvas"
                    }
                  >
                    {t(`nav.${item.labelKey}`)}
                  </Link>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
    </nav>
  );
}
