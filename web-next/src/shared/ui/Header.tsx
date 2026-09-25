"use client";

import { useTranslations } from "next-intl";
import { ModeToggle } from "@/shared/ui/ModeToggle";
import { SearchIcon } from "@/shared/ui/icons";
import { useLinks } from "@/shared/api/network-client";

type LinkState = "connected" | "down" | "unknown";

const LINK_CLASSES: Record<LinkState, string> = {
  connected: "bg-ok-soft text-ok",
  down: "bg-bad-soft text-bad",
  unknown: "bg-warn-soft text-warn",
};

const LINK_LABEL_KEYS: Record<LinkState, string> = {
  connected: "linkConnected",
  down: "linkDown",
  unknown: "linkUnknown",
};

export function Header() {
  const t = useTranslations("header");
  const { data: links } = useLinks();

  let state: LinkState = "unknown";
  if (links !== undefined && links.length > 0) {
    state = links.some((link) => link.status === "SIGNED_ON" || link.status === "CONNECTED")
      ? "connected"
      : "down";
  }

  return (
    <header className="flex h-17 shrink-0 items-center gap-4 border-b border-border bg-surface px-8">
      <label className="flex h-10.5 max-w-115 flex-1 items-center gap-2.5 rounded-[10px] border border-border bg-canvas px-3.5 text-muted">
        <span className="sr-only">{t("search")}</span>
        <SearchIcon className="shrink-0" />
        <input className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none" placeholder={t("search")} />
        <kbd className="shrink-0 rounded-md border border-[#D6D3CA] px-1.5 py-0.5 font-mono text-xs">⌘K</kbd>
      </label>
      <div className="flex-1" />
      <div
        role="status"
        data-link-state={state}
        className={`flex h-8.5 items-center gap-2 rounded-full px-3.5 text-[13px] font-semibold ${LINK_CLASSES[state]}`}
      >
        <span aria-hidden className="relative size-2 shrink-0">
          <span className="absolute inset-0 animate-ping rounded-full bg-current motion-reduce:animate-none" />
          <span className="absolute inset-0 rounded-full bg-current" />
        </span>
        {t(LINK_LABEL_KEYS[state])}
      </div>
      <ModeToggle />
    </header>
  );
}
