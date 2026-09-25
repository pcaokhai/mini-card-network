import type { Transaction } from "@/shared/api/pos-client";
import { type EntryId, MTI, type Outcome, type ResultKind, kindOf, outcomeOf, techLine } from "./pos-model";

export type PosResult =
  | { kind: "tx"; tx: Transaction; last4: string; entry: EntryId; balance: number | null }
  | { kind: "invalidAmount" }
  | { kind: "requestFailed"; detail: string };

export type StepKind = ResultKind | "warn";
export interface ResultStep {
  title: string;
  desc: string;
  kind: StepKind;
}

/** Everything the result panel and the terminal screen render, already in the viewer's language. */
export interface ResultView {
  kind: ResultKind;
  title: string;
  desc: string;
  screen: string;
  tech: string;
  steps: ResultStep[];
  rrn: string | null;
}

type Translate = (key: string, values?: Record<string, string | number>) => string;

const formatVnd = (amount: number) => amount.toLocaleString("vi-VN");

type MakeStep = (key: string, kind: StepKind, desc?: string) => ResultStep;

function stepMaker(t: Translate, values?: Record<string, string>): MakeStep {
  return (key, kind, desc) => ({ title: t(`step.${key}`), desc: desc ?? t(`step.${key}Desc`, values), kind });
}

/** `t` is scoped to the `pos.result` namespace. */
export function describeResult(result: PosResult, t: Translate): ResultView {
  if (result.kind === "invalidAmount") {
    return {
      kind: "bad",
      title: t("title.invalidAmount"),
      desc: t("desc.invalidAmount"),
      screen: t("screenInvalid"),
      tech: t("tech.localCheck"),
      steps: [stepMaker(t)("amountCheck", "bad")],
      rrn: null,
    };
  }
  if (result.kind === "requestFailed") {
    return {
      kind: "bad",
      title: t("title.requestFailed"),
      desc: t("desc.requestFailed", { detail: result.detail }),
      screen: t("screenDeclined"),
      tech: result.detail,
      steps: [],
      rrn: null,
    };
  }
  return describeTransaction(result, t);
}

function describeTransaction(
  { tx, last4, entry, balance }: Extract<PosResult, { kind: "tx" }>,
  t: Translate,
): ResultView {
  const outcome = outcomeOf(tx);
  const kind = kindOf(outcome);
  const knownBalance = tx.type === "BALANCE" ? (tx.balance?.amount ?? null) : balance;
  const values = {
    type: tx.type,
    amount: formatVnd(tx.amount.amount),
    balance: knownBalance === null ? "none" : formatVnd(knownBalance),
    rrn: tx.rrn,
    rc: tx.responseCode ?? "—",
  };
  const step = stepMaker(t, values);
  const posSent =
    tx.type === "COMPLETION"
      ? step("posSent", "ok", t("step.posSentCompletionDesc", { rrn: tx.originalRrn ?? tx.rrn }))
      : step("posSent", "ok", t("step.posSentDesc", { entry, last4 }));
  const acquirer = step("acquirer", "ok");
  return {
    kind,
    title: t(`title.${outcome}`, values),
    desc: t(`desc.${outcome}`, values),
    screen: t(screenKey(outcome)),
    tech: outcome === "rc91" ? t("tech.linkDown", { request: MTI[tx.type][0] }) : techLine(tx),
    steps: stepsFor(outcome, kind, { posSent, acquirer, step, why: () => whyOf(outcome, t, values) }),
    rrn: tx.rrn,
  };
}

function stepsFor(
  outcome: Outcome,
  kind: ResultKind,
  s: { posSent: ResultStep; acquirer: ResultStep; step: MakeStep; why: () => string },
): ResultStep[] {
  if (outcome === "rc91") return [s.posSent, s.step("linkCheck", "bad"), s.step("returnBad", "bad")];
  if (outcome === "reversed" || outcome === "reversalPending") {
    const last = outcome === "reversed" ? s.step("reversed", "rev") : s.step("reversalQueued", "rev");
    return [s.posSent, s.acquirer, s.step("noResponse", "warn"), last];
  }
  const issuer = s.step("issuerCheck", kind, s.why());
  return [s.posSent, s.acquirer, issuer, kind === "ok" ? s.step("returnOk", "ok") : s.step("returnBad", "bad")];
}

function whyOf(outcome: Outcome, t: Translate, values: Record<string, string>): string {
  const known = ["approved", "rc51", "rc54", "rc61", "rc62"].includes(outcome);
  return t(`why.${known ? outcome : "declined"}`, values);
}

function screenKey(outcome: Outcome): string {
  if (outcome === "approved") return "screenOk";
  if (outcome === "rc91" || outcome === "reversed" || outcome === "reversalPending") return "screenLink";
  return "screenDeclined";
}
