"use client";

import { useTranslations } from "next-intl";
import type { Detail } from "./lab-model";

/** MCN-104-AC3: the dark panel explaining the selected MTI, bitmap or field. */
export function DetailPanel({ detail }: { detail: Detail }) {
  const t = useTranslations("lab.detail");
  return (
    <section aria-label={t("label")} aria-live="polite" className="lab-detail">
      <div className="flex items-center gap-2.5">
        <span className="lab-detail__tag">{detail.tag}</span>
        <span className="lab-detail__status" data-on={detail.on}>
          {detail.status}
        </span>
      </div>
      <h2 className="m-0 text-[20px] font-bold">{detail.title}</h2>
      <p className="lab-detail__why">{detail.why}</p>
      <dl className="lab-detail__facts">
        <div className="lab-detail__fact">
          <dt>{t("tech")}</dt>
          <dd>{detail.tech}</dd>
        </div>
        <div className="lab-detail__fact">
          <dt>{t("format")}</dt>
          <dd data-mono="true">{detail.fmt}</dd>
        </div>
        <div className="lab-detail__fact">
          <dt>{t("value")}</dt>
          <dd data-mono="true" title={detail.value}>
            {detail.value}
          </dd>
        </div>
      </dl>
      {detail.parts.length > 0 && (
        <div className="lab-detail__parts">
          {detail.parts.map((part) => (
            <div key={part.k} className="lab-detail__part">
              <span className="lab-detail__part-k">{part.k}</span>
              <span className="lab-detail__part-v">{part.v}</span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
