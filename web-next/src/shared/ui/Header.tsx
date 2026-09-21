"use client";

import { useTranslations } from "next-intl";
import { ModeToggle } from "@/shared/ui/ModeToggle";

export function Header() {
  const t = useTranslations("header");
  return (
    <header className="flex h-17 shrink-0 items-center gap-4 border-b border-border bg-surface px-8">
      <label className="flex h-10.5 max-w-115 flex-1 items-center rounded-[10px] border border-border bg-canvas px-3.5">
        <span className="sr-only">{t("search")}</span>
        <input className="flex-1 bg-transparent text-sm outline-none" placeholder={t("search")} />
      </label>
      <div className="flex-1" />
      {/* Link status becomes live in MCN-204/205. */}
      <div role="status" className="flex h-8.5 items-center gap-2 rounded-full bg-warn-soft px-3.5 text-[13px] font-semibold text-warn">
        {t("linkUnknown")}
      </div>
      <ModeToggle />
    </header>
  );
}
