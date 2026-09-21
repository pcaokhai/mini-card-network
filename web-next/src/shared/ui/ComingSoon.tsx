import { useTranslations } from "next-intl";

export function ComingSoon({ release }: { release: string }) {
  const t = useTranslations("comingSoon");
  return (
    <div className="rounded-card border border-border bg-surface p-8">
      <h1 className="text-2xl font-bold">{t("title", { release })}</h1>
      <p className="mt-2 text-muted">{t("body")}</p>
    </div>
  );
}
