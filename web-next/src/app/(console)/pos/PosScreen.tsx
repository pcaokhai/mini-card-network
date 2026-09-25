"use client";

import { useCallback, useState } from "react";
import { useTranslations } from "next-intl";
import { useQueryClient } from "@tanstack/react-query";
import { CardPicker } from "@/components/pos/CardPicker";
import type { KeypadKey } from "@/components/pos/Keypad";
import { PosDevice } from "@/components/pos/PosDevice";
import { ResultPanel } from "@/components/pos/ResultPanel";
import { ReadingPanel } from "@/components/pos/ReadingPanel";
import type { Scenario } from "@/components/pos/ScenarioButtons";
import { TransactionTypeSelector } from "@/components/pos/TransactionTypeSelector";
import {
  type CardToken,
  DISPLAY_CARDS,
  ENTRY_MODES,
  type EntryId,
  type ScenarioId,
  type TransactionType,
  nextAmount,
} from "@/components/pos/pos-model";
import { type PosResult, describeResult } from "@/components/pos/result-view";
import { useCard } from "@/shared/api/cards-client";
import {
  type Transaction,
  useCreateBalanceInquiry,
  useCreateCompletion,
  useCreatePreAuth,
  useCreatePurchase,
  useCreateRefund,
} from "@/shared/api/pos-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import "@/components/pos/pos.css";

// contracts/fixtures/cards.json terminal 00000042.
const TERMINAL = { terminalId: "00000042", merchantName: "Cà phê Góc Phố" };
const CURRENCY = "704";

interface Draft {
  type: TransactionType;
  cardToken: CardToken;
  entry: EntryId;
  amount: number;
  rrn: string;
}

function useSubmitTransaction() {
  const purchase = useCreatePurchase();
  const preAuth = useCreatePreAuth();
  const completion = useCreateCompletion();
  const refund = useCreateRefund();
  const balance = useCreateBalanceInquiry();
  const pending = [purchase, preAuth, completion, refund, balance].some((m) => m.isPending);

  function submit(draft: Draft): Promise<Transaction> {
    const money = { amount: draft.amount, currency: CURRENCY };
    if (draft.type === "COMPLETION") return completion.mutateAsync({ rrn: draft.rrn, amount: money });
    // No PIN block: the gateway does not forward one yet (risk R-12, plan Ruling R1).
    const entryMode = ENTRY_MODES.find((e) => e.id === draft.entry)?.entryMode ?? "CHIP_NO_PIN";
    const card = { terminalId: TERMINAL.terminalId, cardToken: draft.cardToken, entryMode };
    if (draft.type === "BALANCE") return balance.mutateAsync(card);
    const mutation = { PURCHASE: purchase, PREAUTH: preAuth, REFUND: refund }[draft.type];
    return mutation.mutateAsync({ ...card, amount: money });
  }
  return { submit, pending };
}

function problemDetail(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "object" && error !== null) {
    const problem = error as { detail?: unknown; title?: unknown };
    return String(problem.detail ?? problem.title ?? JSON.stringify(error));
  }
  return String(error);
}

export function PosScreen() {
  const t = useTranslations("pos");
  const expert = useDisplayMode((s) => s.mode === "expert");
  const queryClient = useQueryClient();
  const { submit, pending } = useSubmitTransaction();
  const [type, setType] = useState<TransactionType>("PURCHASE");
  const [cardToken, setCardToken] = useState<CardToken>("tok_normal");
  const [entry, setEntry] = useState<EntryId>("chip");
  const [amount, setAmount] = useState("250000");
  const [scenario, setScenario] = useState<ScenarioId | null>("normal");
  const [rrn, setRrn] = useState("");
  const [result, setResult] = useState<PosResult | null>(null);

  const isBalance = type === "BALANCE";
  const isCompletion = type === "COMPLETION";

  const pressKey = useCallback((key: KeypadKey) => {
    setAmount((current) => nextAmount(current, key));
    setResult(null);
  }, []);

  function pickScenario(picked: Scenario) {
    setType("PURCHASE");
    setScenario(picked.id);
    setCardToken(picked.cardToken);
    setAmount(String(picked.amount));
    setResult(null);
  }

  async function pay() {
    if (pending) return;
    setResult(null);
    if (!isBalance && Number(amount) <= 0) return setResult({ kind: "invalidAmount" });
    try {
      const tx = await submit({ type, cardToken, entry, amount: Number(amount), rrn });
      await queryClient.invalidateQueries({ queryKey: ["cards"] });
      setResult({ kind: "tx", tx, entry, last4: tx.maskedPan.slice(-4), balance: null });
      if (tx.type === "PREAUTH" && tx.status === "APPROVED") setRrn(tx.rrn);
    } catch (error) {
      setResult({ kind: "requestFailed", detail: problemDetail(error) });
    }
  }

  // The result reads the paid card's balance live, so it follows the tiles' refetches.
  const selectedCard = DISPLAY_CARDS.find((c) => c.cardToken === cardToken) ?? DISPLAY_CARDS[0];
  const liveBalance = useCard(selectedCard.cardRef).data?.card?.availableBalance?.amount;
  const shownResult: PosResult | null =
    result?.kind === "tx" && result.last4 === selectedCard.last4 ? { ...result, balance: liveBalance ?? null } : result;
  const view = shownResult && describeResult(shownResult, (key, values) => t(`result.${key}`, values));
  const balanceShown = result?.kind === "tx" ? result.tx.balance?.amount : undefined;
  return (
    <section aria-labelledby="pos-heading" className="pos-root flex flex-col gap-[22px]">
      <div>
        <h1 id="pos-heading" className="text-[30px] font-bold tracking-[-0.01em]">
          {t("title")}
        </h1>
        <p className="mt-1.5 text-[15px] text-muted">{t("subtitle")}</p>
      </div>
      <div className="flex">
        <TransactionTypeSelector
          value={type}
          onChange={(next) => {
            setType(next);
            setResult(null);
          }}
        />
      </div>
      <div className="pos-columns">
        <PosDevice
          {...TERMINAL}
          label={isBalance ? t("device.balance") : t("device.amount")}
          amountText={
            isBalance
              ? balanceShown === undefined
                ? "—"
                : balanceShown.toLocaleString("vi-VN")
              : Number(amount).toLocaleString("vi-VN")
          }
          message={view?.screen ?? t("device.idle")}
          messageKind={view?.kind ?? null}
          processing={pending}
          keypadDisabled={isBalance}
          payDisabled={pending || (isCompletion && rrn.length === 0)}
          onKey={pressKey}
          onPay={() => void pay()}
        />
        <div className="flex min-w-0 flex-col gap-[18px]">
          <CardPicker
            selected={cardToken}
            onSelect={(token) => {
              setCardToken(token);
              setResult(null);
            }}
          />
          <ReadingPanel
            expert={expert}
            isCompletion={isCompletion}
            entry={entry}
            onEntry={setEntry}
            rrn={rrn}
            onRrn={setRrn}
            scenario={scenario}
            onScenario={pickScenario}
          />
          <ResultPanel processing={pending} result={shownResult} expert={expert} type={type} />
        </div>
      </div>
    </section>
  );
}
