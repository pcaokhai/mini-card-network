import { useTranslations } from "next-intl";
import type { ClearingFile } from "@/shared/api/settlement-client";
import { formatMoney } from "@/shared/format/money";
import { formatCount, shortHash } from "./settlement-model";

/** MCN-705-AC1: the clearing file once step 4 has produced it. */
export function ClearingFileCard({ file, expert }: { file: ClearingFile | null | undefined; expert: boolean }) {
  const t = useTranslations("settlement.file");
  const records = file ? formatCount(file.recordCount) : "";
  return (
    <section aria-label={t("label")} className="set-card set-file">
      <h2 className="set-card__title">{t("title")}</h2>
      {file ? (
        <>
          <div className="set-file__doc">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" className="shrink-0 text-accent">
              <path d="M6 3h8l4 4v14H6z" />
              <path d="M14 3v4h4M9 13h6M9 17h6" />
            </svg>
            <div className="set-file__meta">
              <span className="set-file__name">{file.fileName}</span>
              <span className="set-file__summary">{t("summary", { records, total: formatMoney(file.total) })}</span>
            </div>
          </div>
          <dl className="set-file__rows">
            <div className="set-file__row">
              <dt>{t("records")}</dt>
              <dd>{t("recordsValue", { records })}</dd>
            </div>
            <div className="set-file__row">
              <dt>{t("total")}</dt>
              {/* The file's trailer carries minor units, so no currency sign here. */}
              <dd>{formatCount(file.total.amount)}</dd>
            </div>
            <div className="set-file__row">
              <dt>{t(expert ? "hashExpert" : "hashEasy")}</dt>
              <dd title={file.sha256}>{shortHash(file.sha256)}</dd>
            </div>
          </dl>
        </>
      ) : (
        <p className="set-empty">{t("empty")}</p>
      )}
    </section>
  );
}
