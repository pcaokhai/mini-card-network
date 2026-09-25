"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";

const KEYS = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "C", "0", "⌫"] as const;
export type KeypadKey = (typeof KEYS)[number];

const KEY_CLASS: Partial<Record<KeypadKey, string>> = { C: "pos-key pos-key--clear", "⌫": "pos-key pos-key--back" };
const ARIA_KEY: Partial<Record<KeypadKey, "clear" | "backspace">> = { C: "clear", "⌫": "backspace" };

function isTyping(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName));
}

/** The canvas terminal keypad (MCN-305-AC1): real buttons plus physical digit and Backspace keys. */
export function Keypad({ onKey, disabled = false }: { onKey: (key: KeypadKey) => void; disabled?: boolean }) {
  const t = useTranslations("pos.device");

  useEffect(() => {
    if (disabled) return;
    function handleKeydown(event: KeyboardEvent) {
      if (isTyping(event.target) || event.metaKey || event.ctrlKey || event.altKey) return;
      if (/^[0-9]$/.test(event.key)) onKey(event.key as KeypadKey);
      else if (event.key === "Backspace") onKey("⌫");
    }
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  }, [disabled, onKey]);

  return (
    <div role="group" aria-label={t("keypad")} className="pos-keypad">
      {KEYS.map((key) => {
        const aria = ARIA_KEY[key];
        return (
          <button
            key={key}
            type="button"
            disabled={disabled}
            aria-label={aria && t(aria)}
            onClick={() => onKey(key)}
            className={KEY_CLASS[key] ?? "pos-key"}
          >
            {key}
          </button>
        );
      })}
    </div>
  );
}
