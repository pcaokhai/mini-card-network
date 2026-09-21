"use client";

import { useTranslations } from "next-intl";
import { useDisplayMode } from "@/shared/state/display-mode";
import { useMessageLab } from "@/shared/state/message-lab";

export function FieldTable() {
  const decoded = useMessageLab((s) => s.decoded);
  const selectedKey = useMessageLab((s) => s.selectedKey);
  const select = useMessageLab((s) => s.select);
  const mode = useDisplayMode((s) => s.mode);
  const t = useTranslations("lab");
  if (!decoded) return null;
  return (
    <table className="w-full text-sm">
      <thead>
        <tr>
          <th scope="col">{t("de")}</th>
          <th scope="col">{t("name")}</th>
          {mode === "expert" && <th scope="col">{t("format")}</th>}
          <th scope="col">{t("value")}</th>
        </tr>
      </thead>
      <tbody>
        {decoded.fields.map((f) => (
          <tr
            key={f.de}
            onClick={() => select(f.de)}
            aria-current={selectedKey === f.de}
            className={selectedKey === f.de ? "bg-accent-soft" : "cursor-pointer hover:bg-canvas"}
          >
            <td>{f.de}</td>
            <td>{mode === "easy" ? f.easyName : f.technicalName}</td>
            {mode === "expert" && <td>{f.format}</td>}
            <td>{f.value}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
