"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { BitmapPanel } from "@/components/lab/BitmapPanel";
import { DetailPanel } from "@/components/lab/DetailPanel";
import { FieldList } from "@/components/lab/FieldList";
import { RawMessage } from "@/components/lab/RawMessage";
import { type Copy, defaultSelection, describeSelection, fieldRows, rawSegments } from "@/components/lab/lab-model";
import { type DecodedMessage, useDecodedMessage, useSampleMessages } from "@/shared/api/lab-client";
import { useDisplayMode } from "@/shared/state/display-mode";
// globals.css loads JetBrains Mono 400 only; the canvas sets bit numbers, hex and values in 500.
import "@fontsource/jetbrains-mono/500.css";
import "@/components/lab/lab.css";

function problemDetail(error: unknown): string {
  if (error instanceof Error) return error.message;
  const problem = (error ?? {}) as { detail?: unknown; title?: unknown };
  return String(problem.detail ?? problem.title ?? "");
}

export function MessageLabScreen() {
  const t = useTranslations("lab");
  const copy: Copy = { t: (key, values) => t(key, values), has: (key) => t.has(key) };
  const expert = useDisplayMode((s) => s.mode === "expert");
  const samples = useSampleMessages();
  const [sampleIndex, setSampleIndex] = useState(0);
  const [page, setPage] = useState<0 | 1>(0);
  const [picked, setPicked] = useState<string | null>(null);
  const { data: decoded, error: decodeError } = useDecodedMessage(samples.data?.[sampleIndex]?.raw);
  const error = samples.error ?? decodeError;

  function pickSample(index: number) {
    setSampleIndex(index);
    setPage(0);
    setPicked(null);
  }

  return (
    <section aria-labelledby="lab-heading" className="lab-root flex flex-col gap-5">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 id="lab-heading" className="text-[30px] font-bold tracking-[-0.01em]">
            {t("title")}
          </h1>
          <p className="mt-1.5 text-[15px] text-muted">{t("subtitle")}</p>
        </div>
        <div role="group" aria-label={t("samplesLabel")} className="lab-seg">
          {samples.data?.map((sample, i) => (
            <button key={sample.raw} type="button" className="lab-seg__btn" aria-pressed={i === sampleIndex} onClick={() => pickSample(i)}>
              {t.has(`samples.${sample.mti}`) ? t(`samples.${sample.mti}`) : sample.label}
            </button>
          ))}
        </div>
      </div>
      {error && (
        <p role="alert" className="text-bad">
          {t("loadFailed", { detail: problemDetail(error) })}
        </p>
      )}
      {!decoded && !error && <p className="text-muted">{t("loading")}</p>}
      {decoded && <Dissection decoded={decoded} expert={expert} copy={copy} page={page} onPage={setPage} picked={picked} onPick={setPicked} />}
    </section>
  );
}

interface DissectionProps {
  decoded: DecodedMessage;
  expert: boolean;
  copy: Copy;
  page: 0 | 1;
  onPage: (page: 0 | 1) => void;
  picked: string | null;
  onPick: (key: string) => void;
}

/** MCN-104-AC1: one selection drives the raw segments, the bitmap grid, the detail panel and the list. */
function Dissection({ decoded, expert, copy, page, onPage, picked, onPick }: DissectionProps) {
  const selected = picked ?? defaultSelection(decoded);
  const easyRows = fieldRows(decoded, false, copy);
  const nameOf = (de: string) => easyRows.find((r) => r.n === de)?.name ?? de;
  return (
    <>
      <RawMessage
        segments={rawSegments(decoded)}
        selected={selected}
        onSelect={onPick}
        nameOf={nameOf}
        note={expert ? copy.t("raw.noteExpert", { count: decoded.fields.length }) : copy.t("raw.noteEasy")}
      />
      <div className="lab-columns">
        <BitmapPanel decoded={decoded} page={page} onPage={onPage} selected={selected} onSelect={onPick} expert={expert} />
        <div className="flex min-w-0 flex-col gap-[18px]">
          <DetailPanel detail={describeSelection(decoded, selected, copy)} />
          <FieldList rows={expert ? fieldRows(decoded, true, copy) : easyRows} expert={expert} selected={selected} onSelect={onPick} />
        </div>
      </div>
    </>
  );
}
