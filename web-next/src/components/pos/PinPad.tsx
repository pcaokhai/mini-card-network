"use client";

import { useState } from "react";
import { Keypad } from "./Keypad";

const PIN_LENGTH = 4;

interface PinPadProps {
  onSubmit: (pin: string) => void;
}

/** AC4: PIN digits never render in clear and are cleared from state right after submit. */
export function PinPad({ onSubmit }: PinPadProps) {
  const [pin, setPin] = useState("");

  function handleConfirm() {
    onSubmit(pin);
    setPin("");
  }

  return (
    <div className="flex flex-col items-center gap-3">
      <div role="status" aria-label="PIN" className="h-6 text-xl tracking-[0.3em] text-ink">
        {"•".repeat(pin.length)}
      </div>
      <Keypad value={pin} onChange={setPin} maxLength={PIN_LENGTH} />
      <button
        type="button"
        onClick={handleConfirm}
        disabled={pin.length !== PIN_LENGTH}
        className="w-full rounded-lg bg-accent py-2 text-sm font-semibold text-white disabled:opacity-40"
      >
        Confirm
      </button>
    </div>
  );
}
