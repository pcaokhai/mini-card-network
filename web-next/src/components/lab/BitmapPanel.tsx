"use client";

import { useTranslations } from "next-intl";
import type { DecodedMessage } from "@/shared/api/lab-client";
import { bitmapRows, isBitOn } from "./lab-model";

// Canvas: each cell starts (row + column) × 16 ms after the first, so the flip sweeps diagonally.
const DIAGONAL_STEP_MS = 16;

interface BitmapPanelProps {
  decoded: DecodedMessage;
  page: 0 | 1;
  onPage: (page: 0 | 1) => void;
  selected: string;
  onSelect: (key: string) => void;
  expert: boolean;
}

/** MCN-104-AC2/AC5: 8×8 bit grid with per-byte hex and binary; cells flip in on every remount. */
export function BitmapPanel({ decoded, page, onPage, selected, onSelect, expert }: BitmapPanelProps) {
  const t = useTranslations("lab.bitmap");
  const hasSecondary = decoded.secondaryBitmap != null;
  const shown = hasSecondary ? page : 0;
  const bitmap = decoded.primaryBitmap + (decoded.secondaryBitmap ?? "");
  const total = expert ? t(hasSecondary ? "total128" : "total64") : t("totalEasy");
  return (
    <section aria-label={t("label")} className="lab-card gap-3.5">
      <div className="lab-card__head">
        <h2 className="lab-card__title">{t("heading")}</h2>
        <div role="group" aria-label={t("pagesLabel")} className="lab-seg">
          {([0, 1] as const).map((p) => (
            <button
              key={p}
              type="button"
              className="lab-seg__btn"
              aria-pressed={shown === p}
              disabled={p === 1 && !hasSecondary}
              onClick={() => onPage(p)}
            >
              {t(p === 0 ? "primary" : "secondary")}
            </button>
          ))}
        </div>
      </div>
      <p className="lab-grid-note">{t(hasSecondary ? "noteSecondary" : "notePrimary")}</p>
      <div className="lab-grid" key={`${bitmap}-${shown}`}>
        {bitmapRows(decoded, shown).map((row, b) => (
          <div key={b} className="lab-grid__row">
            {row.bits.map((n, j) => {
              const on = isBitOn(decoded, n);
              return (
                <button
                  key={n}
                  type="button"
                  className="lab-bit"
                  data-on={on}
                  aria-label={t("cell", { n, on: String(on) })}
                  aria-pressed={selected === String(n)}
                  style={{ animationDelay: `${(b + j) * DIAGONAL_STEP_MS}ms` }}
                  onClick={() => onSelect(String(n))}
                >
                  {n}
                </button>
              );
            })}
            <span className="lab-grid__byte">
              <span className="lab-grid__hex">{row.hex}</span>
              <span className="lab-grid__bin">{row.bin}</span>
            </span>
          </div>
        ))}
      </div>
      <div className="lab-grid__total">
        <span className="text-muted">{total}</span>
        <span className="font-mono font-medium">{bitmap.replace(/(.{2})(?!$)/g, "$1 ")}</span>
      </div>
    </section>
  );
}
