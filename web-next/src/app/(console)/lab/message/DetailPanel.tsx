"use client";

import { useTranslations } from "next-intl";
import { useMessageLab } from "@/shared/state/message-lab";

const ENTRY_MODES: Record<string, string> = {
  "051": "Chip + PIN",
  "052": "Chip, no PIN",
  "011": "Manual entry + PIN",
  "012": "Manual entry, no PIN",
};

function mtiBreakdown(mti: string) {
  const [version, cls, func, origin] = mti.split("");
  return { version, cls, func, origin };
}

export function DetailPanel() {
  const decoded = useMessageLab((s) => s.decoded);
  const selectedKey = useMessageLab((s) => s.selectedKey);
  const t = useTranslations("lab");
  if (!decoded || !selectedKey) return <p className="text-muted">{t("selectHint")}</p>;

  if (selectedKey === "mti") {
    const { version, cls, func, origin } = mtiBreakdown(decoded.mti);
    return (
      <dl>
        <dt>{t("mtiVersion")}</dt>
        <dd>{version}</dd>
        <dt>{t("mtiClass")}</dt>
        <dd>{cls}</dd>
        <dt>{t("mtiFunction")}</dt>
        <dd>{func}</dd>
        <dt>{t("mtiOrigin")}</dt>
        <dd>{origin}</dd>
      </dl>
    );
  }

  const field = decoded.fields.find((f) => f.de === selectedKey);
  if (!field) return <p className="text-muted">{t("selectHint")}</p>;

  if (field.de === "22") {
    return <p>{ENTRY_MODES[field.value] ?? field.value}</p>;
  }
  // DE 55 (EMV TLV) and DE 90 (original data elements) get their own structured breakdown
  // in a later story once real transactions produce non-trivial values for them (E3/E6);
  // for now they render like any other field — the hook point is here (field.de === "55" / "90").
  return (
    <div>
      <p className="font-semibold">{field.easyName}</p>
      <p className="text-muted">{field.technicalName}</p>
      <p className="font-mono">{field.value}</p>
    </div>
  );
}
