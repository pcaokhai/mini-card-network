"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { useDecodeMessage } from "@/shared/api/lab-client";
import { useMessageLab } from "@/shared/state/message-lab";
import { BitmapGrid } from "./BitmapGrid";
import { DetailPanel } from "./DetailPanel";
import { FieldTable } from "./FieldTable";
import { RawSegments } from "./RawSegments";

export function MessageLabScreen() {
  const t = useTranslations("lab");
  const raw = useMessageLab((s) => s.raw);
  const setRaw = useMessageLab((s) => s.setRaw);
  const decoded = useMessageLab((s) => s.decoded);
  const setDecoded = useMessageLab((s) => s.setDecoded);
  const decode = useDecodeMessage();
  const [tab, setTab] = useState<"primary" | "secondary">("primary");

  useEffect(() => {
    if (!raw) return;
    // MCN-104 ruling: decode on every edit, debounced 300ms (not a MCN-004 motion-duration
    // token — this is an input debounce, not an animation timing).
    const id = setTimeout(() => {
      decode.mutate(
        { raw },
        {
          onSuccess: (d) => setDecoded(d),
          onError: () => setDecoded(null),
        },
      );
    }, 300);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [raw]);

  return (
    <div className="grid grid-cols-[1fr_320px] gap-6">
      <div className="space-y-6">
        <textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          placeholder={t("pasteHint")}
          className="w-full rounded-card border border-border bg-surface p-3 font-mono text-sm"
          rows={3}
        />
        {decoded && (
          <>
            <section aria-label={t("rawHeading")} className="rounded-card border border-border bg-surface p-4">
              <h2 className="mb-3 text-base font-semibold">{t("rawHeading")}</h2>
              <RawSegments />
            </section>
            <section aria-label={t("bitmapHeading")} className="rounded-card border border-border bg-surface p-4">
              <h2 className="mb-3 text-base font-semibold">{t("bitmapHeading")}</h2>
              <div role="tablist" hidden={!decoded.secondaryBitmap} className="mb-3">
                <button role="tab" aria-selected={tab === "primary"} onClick={() => setTab("primary")}>
                  {t("primaryBitmap")}
                </button>
                {decoded.secondaryBitmap && (
                  <button role="tab" aria-selected={tab === "secondary"} onClick={() => setTab("secondary")}>
                    {t("secondaryBitmap")}
                  </button>
                )}
              </div>
              <BitmapGrid page={decoded.secondaryBitmap ? tab : "primary"} />
            </section>
            <section aria-label={t("fieldsHeading")} className="rounded-card border border-border bg-surface p-4">
              <h2 className="mb-3 text-base font-semibold">{t("fieldsHeading")}</h2>
              <FieldTable />
            </section>
          </>
        )}
      </div>
      <aside className="rounded-card bg-ink p-4 text-white">
        <DetailPanel />
      </aside>
    </div>
  );
}
