"use client";

import { useTranslations } from "next-intl";
import { type DisplayMode, useDisplayMode } from "@/shared/state/display-mode";

const MODES: readonly DisplayMode[] = ["easy", "expert"];

export function ModeToggle() {
  const { mode, setMode } = useDisplayMode();
  const t = useTranslations("modeToggle");
  return (
    <div role="group" aria-label={t("label")} className="flex gap-0.5 rounded-lg bg-[#EFEDE6] p-[3px]">
      {MODES.map((m) => (
        <button
          key={m}
          type="button"
          aria-pressed={mode === m}
          onClick={() => setMode(m)}
          className={
            mode === m
              ? "h-8 rounded-md bg-surface px-3.5 text-[13px] font-semibold text-ink shadow-sm"
              : "h-8 rounded-md px-3.5 text-[13px] font-medium text-muted"
          }
        >
          {t(m)}
        </button>
      ))}
    </div>
  );
}
