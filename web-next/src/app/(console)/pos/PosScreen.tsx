"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { CardPicker } from "@/components/pos/CardPicker";
import { PinPad } from "@/components/pos/PinPad";
import { ScenarioButtons, SCENARIO_PRESETS, type Scenario } from "@/components/pos/ScenarioButtons";
import { ResultPanel } from "@/components/pos/ResultPanel";
import { encryptPinBlockForSimulator } from "@/components/pos/pinblock";
import { useCreatePurchase, type Transaction } from "@/shared/api/pos-client";
import { useDisplayMode } from "@/shared/state/display-mode";

const TERMINAL_ID = "00000042";

export function PosScreen() {
  const t = useTranslations("pos");
  const { mode } = useDisplayMode();
  const [cardToken, setCardToken] = useState<string | null>(null);
  const [amount, setAmount] = useState("");
  const [pinEntered, setPinEntered] = useState(false);
  const [result, setResult] = useState<Transaction | null>(null);
  const purchase = useCreatePurchase();
  // Kept out of state (never re-rendered) and cleared immediately in handlePay, before the
  // network call, per AC4: PIN digits must not linger in component state.
  const pinRef = useRef<string | null>(null);

  function handlePinSubmit(pin: string) {
    pinRef.current = pin;
    setPinEntered(true);
  }

  function applyScenario(scenario: Scenario) {
    const preset = SCENARIO_PRESETS[scenario];
    setCardToken(preset.cardToken);
    setAmount(String(preset.amount));
  }

  async function handlePay() {
    const pin = pinRef.current;
    if (!cardToken || !pin || !amount) return;
    // Lab simplification: the frontend never holds the real PAN the gateway alone resolves
    // (see docs/plans/MCN-305.md Task 5), so the PIN block is built against a per-card
    // placeholder value derived from cardToken. This demonstrates the real ISO 9564-1 shape
    // (AC4's intent) without shipping real PANs to the browser.
    const encryptedPinBlock = await encryptPinBlockForSimulator(pin, cardToken.padEnd(16, "0").slice(0, 16));
    pinRef.current = null;
    const transaction = await purchase.mutateAsync({
      terminalId: TERMINAL_ID,
      cardToken,
      entryMode: "MANUAL_PIN",
      encryptedPinBlock,
      amount: { amount: Number(amount), currency: "704" },
    });
    setResult(transaction);
  }

  const canPay = Boolean(cardToken && pinEntered && amount) && !purchase.isPending;

  return (
    <section aria-labelledby="pos-heading" className="space-y-6">
      <h1 id="pos-heading" className="text-2xl font-bold">
        {t("title")}
      </h1>
      <ScenarioButtons onPick={applyScenario} />
      <CardPicker selected={cardToken} onSelect={setCardToken} />
      <div>
        <label htmlFor="pos-amount" className="mb-1 block text-sm font-medium text-muted">
          {t("amount")}
        </label>
        <input
          id="pos-amount"
          type="text"
          inputMode="numeric"
          value={amount}
          onChange={(e) => setAmount(e.target.value.replace(/\D/g, ""))}
          className="w-40 rounded-lg border border-border bg-surface px-3 py-2 text-lg"
        />
      </div>
      <PinPad onSubmit={handlePinSubmit} />
      <div aria-live="polite">{purchase.isPending && t("processing")}</div>
      <button
        type="button"
        onClick={handlePay}
        disabled={!canPay}
        className="w-full rounded-lg bg-accent py-2.5 text-sm font-semibold text-white disabled:opacity-40"
      >
        {t("pay")}
      </button>
      <ResultPanel transaction={result} expertMode={mode === "expert"} />
    </section>
  );
}
