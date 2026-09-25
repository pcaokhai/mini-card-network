import { useTranslations } from "next-intl";

// docs/03-iso8583-interface-spec.md §7.6: the business day closes at 23:59:59 local.
const CUTOVER_LABEL = "23:59";

export function BusinessDateCard() {
  const t = useTranslations("sidebar");

  return (
    <div className="mt-auto flex flex-col gap-1 rounded-xl bg-canvas p-3.5">
      <span className="text-xs text-muted">{t("businessDate")}</span>
      {/* The business day is the viewer's local day; the server has no timezone to match it. */}
      <span className="text-[15px] font-semibold" suppressHydrationWarning>
        {new Intl.DateTimeFormat("vi-VN", { day: "2-digit", month: "2-digit", year: "numeric" }).format(new Date())}
      </span>
      <span className="text-xs text-muted">{t("cutoverAt", { time: CUTOVER_LABEL })}</span>
    </div>
  );
}
