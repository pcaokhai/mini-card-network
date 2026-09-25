"use client";

import { useRef, useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { ILLUSTRATION_PAN_MASKED, isValidPin, pinBlockRows } from "./security-model";

// The canvas opens with this PIN already worked through, so the rows are never empty.
const SAMPLE_PIN = "1234";
const ROW_STAGGER_MS = 110;

/** ISO 9564 format 0 worked through in the browser, on a made-up card; the cipher rows are illustrative. */
export function PinBlockVisualiser({ expert, zpkKcv }: { expert: boolean; zpkKcv: string }) {
  const t = useTranslations("security.pinBlock");
  const input = useRef<HTMLInputElement>(null);
  const [pin, setPin] = useState(SAMPLE_PIN);
  const [generation, setGeneration] = useState(0);
  const [hasError, setHasError] = useState(false);

  function build(event: FormEvent) {
    event.preventDefault();
    const candidate = input.current?.value ?? "";
    if (!isValidPin(candidate)) {
      setHasError(true);
      return;
    }
    setPin(candidate);
    setGeneration((g) => g + 1);
  }

  return (
    <section aria-label={t("region")} className="security-panel security-pin">
      <h2 className="security-panel__heading">{t("heading")}</h2>
      <form className="security-pin__form" onSubmit={build} noValidate>
        <div className="security-pin__field">
          <label htmlFor="security-pin-input">{t("pinLabel")}</label>
          <input
            ref={input}
            id="security-pin-input"
            type="password"
            inputMode="numeric"
            autoComplete="off"
            defaultValue={SAMPLE_PIN}
            placeholder={t("pinPlaceholder")}
            aria-invalid={hasError}
            onChange={() => setHasError(false)}
          />
        </div>
        <button type="submit" className="security-button">
          {t("build")}
        </button>
      </form>
      {hasError && (
        <p role="alert" className="security-error">
          {t("pinError")}
        </p>
      )}
      <p className="security-note">{t("card", { maskedPan: ILLUSTRATION_PAN_MASKED })}</p>
      <ol key={generation} className="security-pin__rows">
        {pinBlockRows(pin, zpkKcv).map(({ id, value }, i) => (
          <li key={id} className="security-pin__row" data-row={id} style={{ animationDelay: `${i * ROW_STAGGER_MS}ms` }}>
            <span className="security-pin__label">
              <span className="security-pin__name">{t(`rows.${id}.${expert ? "expert" : "easy"}`)}</span>
              <span className="security-note">{expert ? t(`rows.${id}.easy`) : t(`rows.${id}.hint`)}</span>
            </span>
            <span className="security-pin__value">{value}</span>
          </li>
        ))}
      </ol>
      <p className="security-note security-pin__footnote">{t("footnote")}</p>
    </section>
  );
}
