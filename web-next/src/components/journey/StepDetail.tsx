import { useTranslations } from "next-intl";
import type { components } from "@/shared/api/generated/schema";

type JourneyStep = components["schemas"]["JourneyStep"];

// MCN-104's FieldTable/DetailPanel read from the message-lab zustand store and take no props
// (see web-next/src/app/(console)/lab/message/{FieldTable,DetailPanel}.tsx) — a journey step's
// message comes from this screen's own query result, not that store, so their shape doesn't fit
// here. Render the same IsoField[] shape as a plain table instead of forking their store wiring.
export function StepDetail({ step }: { step: JourneyStep }) {
  const t = useTranslations("journey.detail");
  if (!step.message) return <p className="text-muted">{t("empty")}</p>;

  return (
    <table className="w-full text-sm">
      <thead>
        <tr>
          <th scope="col">{t("de")}</th>
          <th scope="col">{t("name")}</th>
          <th scope="col">{t("value")}</th>
        </tr>
      </thead>
      <tbody>
        {step.message.fields.map((field) => (
          <tr key={field.de}>
            <td className="font-mono">{field.de}</td>
            <td>{field.technicalName}</td>
            <td className="font-mono">{field.value}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
