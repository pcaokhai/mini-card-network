"use client";

import { useTranslations } from "next-intl";
import { TECHNICAL_LABELS, type GlossaryId } from "@/shared/i18n/glossary";
import { useDisplayMode } from "@/shared/state/display-mode";

/** Renders a domain term in the current display mode; the hint explains it in the current locale. */
export function Term({ id }: { id: GlossaryId }) {
  const mode = useDisplayMode((s) => s.mode);
  const t = useTranslations("glossary");
  return (
    <span data-term={id} title={t(`${id}.hint`)} className="cursor-help underline decoration-dotted underline-offset-4">
      {mode === "easy" ? t(`${id}.easy`) : TECHNICAL_LABELS[id]}
    </span>
  );
}
