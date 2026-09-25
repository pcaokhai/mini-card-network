"use client";

import { useTranslations } from "next-intl";
import { ENTRY_MODES, type EntryId, type ScenarioId } from "./pos-model";
import { type Scenario, ScenarioButtons } from "./ScenarioButtons";
import { Segmented } from "./Segmented";

interface ReadingPanelProps {
  expert: boolean;
  isCompletion: boolean;
  entry: EntryId;
  onEntry: (entry: EntryId) => void;
  rrn: string;
  onRrn: (rrn: string) => void;
  scenario: ScenarioId | null;
  onScenario: (scenario: Scenario) => void;
}

/** "Cách đọc thẻ" (or the original RRN for a completion) and the quick scenarios. */
export function ReadingPanel(props: ReadingPanelProps) {
  const t = useTranslations("pos.reading");
  const entryText = (id: EntryId, de22: string) =>
    props.expert ? `${t(`entryMode.${id}`)} · ${de22}` : t(`entryMode.${id}`);
  return (
    <section aria-label={t("sectionLabel")} className="pos-panel">
      {props.isCompletion ? (
        <div className="pos-row">
          <label htmlFor="pos-rrn" className="pos-row__label">
            {t("rrn")}
          </label>
          <input
            id="pos-rrn"
            inputMode="numeric"
            maxLength={12}
            placeholder={t("rrnHint")}
            value={props.rrn}
            onChange={(e) => props.onRrn(e.target.value.replace(/\D/g, ""))}
            className="pos-input"
          />
        </div>
      ) : (
        <div className="pos-row">
          <div className="pos-row__label">{t("entry")}</div>
          <Segmented
            label={t("entry")}
            value={props.entry}
            onChange={props.onEntry}
            options={ENTRY_MODES.map((e) => ({ id: e.id, text: entryText(e.id, e.de22) }))}
          />
        </div>
      )}
      <div className="pos-row">
        <div className="pos-row__label">{t("scenarios")}</div>
        <ScenarioButtons selected={props.scenario} onPick={props.onScenario} />
      </div>
    </section>
  );
}
