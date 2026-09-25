import { useTranslations } from "next-intl";
import type { CardSummary } from "@/shared/api/cards-client";
import type { StatusKind } from "./cards-model";

const TONE: Record<StatusKind, "ok" | "bad" | "info"> = { active: "ok", locked: "bad", expired: "info" };

/** Easy mode names the state; Expert mode shows the issuer's card.status (EXPIRED once past expiry). */
export function StatusBadge({ card, kind, expert }: { card: Pick<CardSummary, "status">; kind: StatusKind; expert: boolean }) {
  const t = useTranslations("cards.status.easy");
  const technical = kind === "expired" ? "EXPIRED" : card.status;
  return (
    <span className="cards-badge" data-tone={TONE[kind]}>
      {expert ? technical : t(kind)}
    </span>
  );
}
