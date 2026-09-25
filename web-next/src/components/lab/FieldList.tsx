"use client";

import { useTranslations } from "next-intl";
import type { FieldRow } from "./lab-model";

interface FieldListProps {
  rows: FieldRow[];
  expert: boolean;
  selected: string;
  onSelect: (key: string) => void;
}

/** MCN-104-AC4: one row per field; Expert mode switches to technical names and adds the format column. */
export function FieldList({ rows, expert, selected, onSelect }: FieldListProps) {
  const t = useTranslations("lab.list");
  return (
    <section aria-label={t("label")} className="lab-fields" data-expert={expert}>
      <div className="lab-fields__head">
        <span>{t("field")}</span>
        <span>{t(expert ? "techName" : "easyName")}</span>
        {expert && <span>{t("format")}</span>}
        <span className="text-right">{t("value")}</span>
      </div>
      {rows.map((row) => (
        <button
          key={row.n}
          type="button"
          className="lab-fields__row"
          aria-pressed={selected === row.n}
          onClick={() => onSelect(row.n)}
        >
          <span className="lab-fields__n">{row.n}</span>
          <span className="lab-fields__name">{row.name}</span>
          {expert && <span className="lab-fields__fmt">{row.fmt}</span>}
          <span className="lab-fields__value">{row.value}</span>
        </button>
      ))}
    </section>
  );
}
