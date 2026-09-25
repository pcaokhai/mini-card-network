"use client";

import { useTranslations } from "next-intl";
import type { Segment } from "./lab-model";

interface RawMessageProps {
  segments: Segment[];
  selected: string;
  onSelect: (key: string) => void;
  nameOf: (de: string) => string;
  note: string;
}

/** MCN-104-AC1: the packed message as coloured, clickable segments (MTI, bitmap, each field). */
export function RawMessage({ segments, selected, onSelect, nameOf, note }: RawMessageProps) {
  const t = useTranslations("lab.raw");
  const hint = (s: Segment) =>
    s.kind === "mti" ? t("hintMti") : s.kind === "bmp" ? t("hintBitmap") : t("hintField", { n: s.key, name: nameOf(s.key) });
  return (
    <section aria-label={t("label")} className="lab-card gap-3">
      <div className="lab-card__head">
        <h2 className="lab-card__title">{t("heading")}</h2>
        <div className="lab-legend">
          {(["mti", "bmp", "field"] as const).map((kind) => (
            <span key={kind} className="lab-legend__item">
              <span className="lab-legend__swatch" data-kind={kind} />
              {t(kind === "mti" ? "legendMti" : kind === "bmp" ? "legendBitmap" : "legendFields")}
            </span>
          ))}
        </div>
      </div>
      <div className="lab-raw">
        {segments.map((s) => (
          <button
            key={s.key}
            type="button"
            className="lab-raw__seg"
            data-kind={s.kind}
            title={hint(s)}
            aria-pressed={selected === s.key}
            onClick={() => onSelect(s.key)}
          >
            {s.text}
          </button>
        ))}
      </div>
      <p className="lab-note">{note}</p>
    </section>
  );
}
