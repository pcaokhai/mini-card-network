"use client";

import { useEffect } from "react";

const DIGITS = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"] as const;

interface KeypadProps {
  value: string;
  onChange: (value: string) => void;
  maxLength: number;
}

/** Keyboard-accessible numeric entry (MCN-305-AC1): real <button>s plus physical keydown support. */
export function Keypad({ value, onChange, maxLength }: KeypadProps) {
  useEffect(() => {
    function handleKeydown(event: KeyboardEvent) {
      if (/^[0-9]$/.test(event.key) && value.length < maxLength) {
        onChange(value + event.key);
      } else if (event.key === "Backspace") {
        onChange(value.slice(0, -1));
      }
    }
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  }, [value, maxLength, onChange]);

  function appendDigit(digit: string) {
    if (value.length >= maxLength) return;
    onChange(value + digit);
  }

  return (
    <div className="grid grid-cols-3 gap-2" role="group" aria-label="Keypad">
      {DIGITS.map((digit) => (
        <button
          key={digit}
          type="button"
          onClick={() => appendDigit(digit)}
          className="rounded-lg border border-border bg-surface py-3 text-lg font-semibold text-ink hover:bg-canvas"
        >
          {digit}
        </button>
      ))}
      <button
        type="button"
        aria-label="Backspace"
        onClick={() => onChange(value.slice(0, -1))}
        className="rounded-lg border border-border bg-surface py-3 text-sm font-semibold text-muted hover:bg-canvas"
      >
        ⌫
      </button>
    </div>
  );
}
