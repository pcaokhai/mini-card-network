import { useLocale, useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { Journey } from "@/shared/api/journey-client";
import { useDisplayMode } from "@/shared/state/display-mode";
import type { Outcome } from "./journey-model";

type Step = Journey["steps"][number];

const MS_PER_SECOND = 1000;
const REPEAT_ADVICE_MTI = "0421";

export interface StepCopy {
  title: string;
  easy: string;
  /** What the timeline shows under the title: Expert reads the provider's technical line. */
  line: string;
  tech: string;
  actor: string;
}

/**
 * Easy/Expert copy for the journey, rendered from each step's language-neutral `code` (contracts
 * StepCode). A step without a code (an older provider) falls back to the provider's English text.
 */
export function useJourneyCopy(journey: Journey) {
  const t = useTranslations("journey");
  const tRc = useTranslations("responseCodes");
  const locale = useLocale();
  const expert = useDisplayMode().mode === "expert";
  const txn = journey.transaction;
  const amount = formatMoney(txn.amount);
  const lastOffset = journey.steps.at(-1)?.offsetMs ?? 0;

  function total(ms: number = lastOffset): string {
    if (ms < MS_PER_SECOND) return `${ms} ms`;
    return t("summary.seconds", { value: (ms / MS_PER_SECOND).toLocaleString(locale, { maximumFractionDigits: 2 }) });
  }

  function key(step: Step, index: number): string | null {
    if (!step.code) return null;
    if (step.code === "REVERSAL_SENT" && step.message?.mti === REPEAT_ADVICE_MTI) return "step.REVERSAL_REPEAT";
    if (step.code !== "POS_RESULT") return `step.${step.code}`;
    if (step.kind === "OK") return "step.POS_RESULT.approved";
    const timedOut = journey.steps.slice(0, index).some((s) => s.code === "NO_RESPONSE" || s.code === "LOCAL_DECLINE");
    return timedOut ? "step.POS_RESULT.failed" : "step.POS_RESULT.declined";
  }

  function step(s: Step, index: number): StepCopy {
    const k = key(s, index);
    const reason = txn.responseCode && tRc.has(txn.responseCode) ? tRc(txn.responseCode) : (txn.responseLabel ?? "");
    // "The whole journey took …" is the time until the POS answered, not until a later reversal.
    const params = { last4: txn.maskedPan.slice(-4), merchant: txn.merchantName, amount, reason, total: total(s.offsetMs) };
    // A code this build has no copy for (a newer provider) falls back to the provider's text (JRN-G11).
    const known = k !== null && t.has(`${k}.title`);
    const title = known ? t(`${k}.title`) : s.title;
    const easy = known ? t(`${k}.easy`, params) : s.easyText;
    return {
      title,
      easy,
      tech: s.technicalText,
      line: expert ? s.technicalText : easy,
      actor: expert ? t(`actorExpert.${s.actor}`) : t(`actor.${s.actor}`),
    };
  }

  function summary(outcome: Outcome) {
    const type = t.has(`summary.type.${txn.type}`) ? t(`summary.type.${txn.type}`) : txn.type;
    const rrn = txn.rrn;
    return {
      headline: t(`summary.headline.${outcome}`, { type, amount }),
      meta: [txn.merchantName, t("summary.card", { last4: txn.maskedPan.slice(-4) }), expert ? t("summary.rrnExpert", { rrn }) : t("summary.rrnEasy", { rrn })].join(" · "),
      total: total(),
      totalLabel: expert ? t("summary.totalExpert") : t("summary.total"),
      messages: journey.steps.filter((s) => s.message).length,
      messagesLabel: expert ? t("summary.messagesExpert") : t("summary.messages"),
      money: t(`summary.moneyValue.${outcome}`, { amount }),
    };
  }

  return { step, summary, expert, locale };
}
