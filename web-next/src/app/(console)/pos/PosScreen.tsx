"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { CardPicker } from "@/components/pos/CardPicker";
import { PinPad } from "@/components/pos/PinPad";
import { ScenarioButtons, SCENARIO_PRESETS, type Scenario } from "@/components/pos/ScenarioButtons";
import { ResultPanel } from "@/components/pos/ResultPanel";
import { TransactionTypeSelector, type TransactionType } from "@/components/pos/TransactionTypeSelector";
import { encryptPinBlockForSimulator } from "@/components/pos/pinblock";
import {
  useCreateBalanceInquiry,
  useCreateCompletion,
  useCreatePreAuth,
  useCreatePurchase,
  useCreateRefund,
  type Transaction,
} from "@/shared/api/pos-client";
import { useDisplayMode } from "@/shared/state/display-mode";

const TERMINAL_ID = "00000042";

export function PosScreen() {
  const t = useTranslations("pos");
  const { mode } = useDisplayMode();
  const [txnType, setTxnType] = useState<TransactionType>("PURCHASE");
  const [cardToken, setCardToken] = useState<string | null>(null);
  const [amount, setAmount] = useState("");
  const [rrn, setRrn] = useState("");
  const [pinEntered, setPinEntered] = useState(false);
  const [result, setResult] = useState<Transaction | null>(null);
  const purchase = useCreatePurchase();
  const preAuth = useCreatePreAuth();
  const completion = useCreateCompletion();
  const refund = useCreateRefund();
  const balanceInquiry = useCreateBalanceInquiry();
  const pending =
    purchase.isPending || preAuth.isPending || completion.isPending || refund.isPending || balanceInquiry.isPending;
  // Kept out of state (never re-rendered) and cleared immediately in handleSubmit, before the
  // network call, per AC4: PIN digits must not linger in component state.
  const pinRef = useRef<string | null>(null);

  const isCompletion = txnType === "COMPLETION";
  const needsAmount = txnType !== "BALANCE";
  const needsCardPresent = !isCompletion;

  function handlePinSubmit(pin: string) {
    pinRef.current = pin;
    setPinEntered(true);
  }

  function applyScenario(scenario: Scenario) {
    const preset = SCENARIO_PRESETS[scenario];
    setCardToken(preset.cardToken);
    setAmount(String(preset.amount));
  }

  async function buildCardPresentData() {
    const pin = pinRef.current;
    if (!cardToken || !pin) return null;
    // Lab simplification: the frontend never holds the real PAN the gateway alone resolves
    // (see docs/plans/MCN-305.md Task 5), so the PIN block is built against a per-card
    // placeholder value derived from cardToken. This demonstrates the real ISO 9564-1 shape
    // (AC4's intent) without shipping real PANs to the browser.
    const encryptedPinBlock = await encryptPinBlockForSimulator(pin, cardToken.padEnd(16, "0").slice(0, 16));
    pinRef.current = null;
    return { terminalId: TERMINAL_ID, cardToken, entryMode: "MANUAL_PIN" as const, encryptedPinBlock };
  }

  async function handleSubmit() {
    if (isCompletion) {
      if (!rrn || !amount) return;
      const transaction = await completion.mutateAsync({ rrn, amount: { amount: Number(amount), currency: "704" } });
      setResult(transaction);
      return;
    }
    const cardPresent = await buildCardPresentData();
    if (!cardPresent) return;
    if (txnType === "BALANCE") {
      const transaction = await balanceInquiry.mutateAsync(cardPresent);
      setResult(transaction);
      return;
    }
    if (!amount) return;
    const withAmount = { ...cardPresent, amount: { amount: Number(amount), currency: "704" } };
    const transaction = await (txnType === "PREAUTH"
      ? preAuth.mutateAsync(withAmount)
      : txnType === "REFUND"
        ? refund.mutateAsync(withAmount)
        : purchase.mutateAsync(withAmount));
    setResult(transaction);
  }

  const canSubmit = isCompletion
    ? Boolean(rrn && amount)
    : Boolean(cardToken && pinEntered && (!needsAmount || amount)) && !pending;

  return (
    <section aria-labelledby="pos-heading" className="space-y-6">
      <h1 id="pos-heading" className="text-2xl font-bold">
        {t("title")}
      </h1>
      <TransactionTypeSelector value={txnType} onChange={setTxnType} />
      <section aria-label={t("cardSectionLabel")} className="rounded-card border border-border bg-surface p-5">
        {isCompletion ? (
          <div>
            <label htmlFor="pos-rrn" className="mb-1 block text-sm font-medium text-muted">
              {t("rrn")}
            </label>
            <input
              id="pos-rrn"
              type="text"
              value={rrn}
              onChange={(e) => setRrn(e.target.value)}
              className="w-64 rounded-lg border border-border bg-surface px-3 py-2 text-lg"
            />
          </div>
        ) : (
          <div className="space-y-4">
            <ScenarioButtons onPick={applyScenario} />
            <CardPicker selected={cardToken} onSelect={setCardToken} />
          </div>
        )}
      </section>
      <section aria-label={t("paymentSectionLabel")} className="rounded-card border border-border bg-surface p-5 space-y-4">
        {needsAmount && (
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
        )}
        {needsCardPresent && <PinPad onSubmit={handlePinSubmit} />}
        <div aria-live="polite">{pending && t("processing")}</div>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={!canSubmit}
          className="w-full rounded-lg bg-accent py-2.5 text-sm font-semibold text-white disabled:opacity-40"
        >
          {t("pay")}
        </button>
      </section>
      <ResultPanel transaction={result} expertMode={mode === "expert"} />
    </section>
  );
}
