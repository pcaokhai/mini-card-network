"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { buildPinBlock } from "@/components/pos/pinblock";

const MIN_PIN_LENGTH = 4;
const MAX_PIN_LENGTH = 12;

// contracts/fixtures/cards.json's "tok_normal" test card — never a real or live PAN.
const TEST_PAN = "9704360000004417";
const TEST_PAN_MASKED = `${TEST_PAN.slice(0, 6)}••••••${TEST_PAN.slice(-4)}`;

export function PinBlockVisualiser() {
  const t = useTranslations("security.pinBlock");
  const [pin, setPin] = useState("");

  const isValid = pin.length >= MIN_PIN_LENGTH && pin.length <= MAX_PIN_LENGTH;
  const pinBlockHex = isValid ? buildPinBlock(pin, TEST_PAN) : null;

  return (
    <section aria-labelledby="pin-block-heading" className="space-y-3">
      <h3 id="pin-block-heading" className="text-sm font-semibold">
        {t("heading")}
      </h3>
      <p className="text-xs text-muted">{t("testCard", { maskedPan: TEST_PAN_MASKED })}</p>
      <div>
        <label htmlFor="pin-block-input" className="mb-1 block text-xs text-muted">
          {t("pinLabel")}
        </label>
        <input
          id="pin-block-input"
          type="text"
          inputMode="numeric"
          value={pin}
          onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))}
          className="w-40 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
        />
        {pin.length > 0 && !isValid && (
          <p role="alert" className="mt-1 text-xs font-medium text-bad">
            {t("pinError")}
          </p>
        )}
      </div>
      {pinBlockHex !== null && (
        <div>
          <p data-testid="pin-block-hex" className="font-mono text-sm">
            {pinBlockHex}
          </p>
          <p className="mt-1 text-xs text-muted">{t("breakdown")}</p>
        </div>
      )}
    </section>
  );
}
